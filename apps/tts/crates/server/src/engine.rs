//! The scheduler: one thread owns the model and advances every admitted
//! stream by one frame per step. Live jobs, where a listener waits, take
//! slots first; background jobs fill what is left and never delay a live
//! one. Admission is decided from the slot counters, so a job that cannot
//! be served is refused at once rather than queued behind three minutes of
//! someone else's reading.

use std::collections::{HashMap, VecDeque};
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::sync::mpsc::{self, Receiver, RecvTimeoutError, Sender};
use std::time::{Duration, Instant};

use anyhow::Result;
use tts_engine::TTSModel;
use tts_engine::batch::{Batcher, StreamId};
use tts_engine::voice_state::ModelState;

use crate::encode::to_i16;
use crate::metrics::Metrics;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Class {
    Live,
    Background,
}

pub struct Job {
    pub segments: Vec<String>,
    pub voice: Arc<ModelState>,
    pub class: Class,
    /// Receives each frame's samples; dropped when the reading is complete.
    pub frames: tokio::sync::mpsc::UnboundedSender<Vec<i16>>,
    pub submitted: Instant,
}

struct Active {
    job: Job,
    segment: usize,
    first_frame_at: Option<Instant>,
}

#[derive(Clone)]
pub struct EngineHandle {
    jobs: Sender<Job>,
    pub metrics: Arc<Metrics>,
    pub live_slots: usize,
    pub background_slots: usize,
    pub sample_rate: u32,
}

impl EngineHandle {
    /// Refuses at once when no slot of the job's class is free, so the caller
    /// can answer with a retry hint instead of holding the connection.
    pub fn submit(&self, job: Job) -> Result<(), Job> {
        let m = &self.metrics;
        let (active, queued, slots) = match job.class {
            Class::Live => (&m.live_active, &m.live_queued, self.live_slots),
            Class::Background => (
                &m.background_active,
                &m.background_queued,
                self.background_slots,
            ),
        };
        if active.load(Ordering::Relaxed) + queued.load(Ordering::Relaxed) >= slots {
            m.rejected.fetch_add(1, Ordering::Relaxed);
            return Err(job);
        }
        queued.fetch_add(1, Ordering::Relaxed);
        m.admitted.fetch_add(1, Ordering::Relaxed);
        self.jobs.send(job).map_err(|e| {
            queued.fetch_sub(1, Ordering::Relaxed);
            e.0
        })
    }
}

pub fn spawn(
    model: TTSModel,
    live_slots: usize,
    background_slots: usize,
    metrics: Arc<Metrics>,
) -> Result<EngineHandle> {
    let sample_rate = model.sample_rate as u32;
    let (tx, rx) = mpsc::channel();
    let handle = EngineHandle {
        jobs: tx,
        metrics: metrics.clone(),
        live_slots,
        background_slots,
        sample_rate,
    };
    let batcher = Batcher::new(model)?;
    std::thread::Builder::new()
        .name("tts-engine".into())
        .spawn(move || run(batcher, rx, live_slots, background_slots, metrics))?;
    Ok(handle)
}

fn run(
    mut batcher: Batcher,
    jobs: Receiver<Job>,
    live_slots: usize,
    background_slots: usize,
    metrics: Arc<Metrics>,
) {
    let mut live: VecDeque<Job> = VecDeque::new();
    let mut background: VecDeque<Job> = VecDeque::new();
    let mut active: HashMap<StreamId, Active> = HashMap::new();
    let sample_rate = batcher.model().sample_rate as f64;

    loop {
        let idle = active.is_empty();
        let incoming = if idle {
            match jobs.recv() {
                Ok(job) => Some(job),
                Err(_) => return,
            }
        } else {
            match jobs.recv_timeout(Duration::ZERO) {
                Ok(job) => Some(job),
                Err(RecvTimeoutError::Timeout) => None,
                Err(RecvTimeoutError::Disconnected) => return,
            }
        };
        if let Some(job) = incoming {
            match job.class {
                Class::Live => live.push_back(job),
                Class::Background => background.push_back(job),
            }
            while let Ok(job) = jobs.try_recv() {
                match job.class {
                    Class::Live => live.push_back(job),
                    Class::Background => background.push_back(job),
                }
            }
        }

        let live_active = active
            .values()
            .filter(|a| a.job.class == Class::Live)
            .count();
        let bg_active = active.len() - live_active;
        let mut live_active = live_active;
        let mut bg_active = bg_active;
        while live_active < live_slots {
            let Some(job) = live.pop_front() else { break };
            metrics.live_queued.fetch_sub(1, Ordering::Relaxed);
            if start(&mut batcher, &mut active, job) {
                live_active += 1;
            }
        }
        while live.is_empty() && bg_active < background_slots {
            let Some(job) = background.pop_front() else {
                break;
            };
            metrics.background_queued.fetch_sub(1, Ordering::Relaxed);
            if start(&mut batcher, &mut active, job) {
                bg_active += 1;
            }
        }
        metrics.live_active.store(live_active, Ordering::Relaxed);
        metrics
            .background_active
            .store(bg_active, Ordering::Relaxed);
        if active.is_empty() {
            continue;
        }

        let t0 = Instant::now();
        let frames = match batcher.step() {
            Ok(frames) => frames,
            Err(e) => {
                tracing::error!(error = %e, "engine step failed; dropping every stream");
                active.clear();
                continue;
            }
        };
        metrics.step_ms.observe(t0.elapsed().as_secs_f64() * 1000.0);

        let mut produced = 0.0;
        for frame in frames {
            let Some(mut a) = active.remove(&frame.id) else {
                continue;
            };
            let samples = match frame.audio.squeeze(0).and_then(|t| t.to_vec1::<f32>()) {
                Ok(s) => s,
                Err(e) => {
                    tracing::error!(error = %e, "bad frame; dropping the stream");
                    continue;
                }
            };
            produced += samples.len() as f64 / sample_rate;
            if a.first_frame_at.is_none() {
                a.first_frame_at = Some(Instant::now());
                metrics
                    .first_audio_ms
                    .observe(a.job.submitted.elapsed().as_secs_f64() * 1000.0);
            }
            if a.job.frames.send(to_i16(&samples)).is_err() {
                if !frame.done {
                    let _ = batcher.remove(frame.id);
                }
                continue;
            }
            if !frame.done {
                active.insert(frame.id, a);
                continue;
            }
            a.segment += 1;
            if a.segment < a.job.segments.len() {
                let text = a.job.segments[a.segment].clone();
                match batcher.add(&text, &a.job.voice) {
                    Ok(id) => {
                        active.insert(id, a);
                    }
                    Err(e) => tracing::error!(error = %e, "could not start the next segment"),
                }
            }
        }
        *metrics.audio_seconds.lock().unwrap() += produced;
        let live_left = active
            .values()
            .filter(|a| a.job.class == Class::Live)
            .count();
        metrics.live_active.store(live_left, Ordering::Relaxed);
        metrics
            .background_active
            .store(active.len() - live_left, Ordering::Relaxed);
    }
}

fn start(batcher: &mut Batcher, active: &mut HashMap<StreamId, Active>, job: Job) -> bool {
    let Some(text) = job.segments.first() else {
        return false;
    };
    match batcher.add(text, &job.voice) {
        Ok(id) => {
            active.insert(
                id,
                Active {
                    job,
                    segment: 0,
                    first_frame_at: None,
                },
            );
            true
        }
        Err(e) => {
            tracing::error!(error = %e, "could not start a reading");
            false
        }
    }
}
