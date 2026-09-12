//! Frame-level batching: every active stream advances by one 80 ms frame per
//! `step`, through a single pass of the flow LM over the stacked rows. The
//! model is bound by memory reads at batch 1, so a step for N streams costs
//! far less than N steps.

use anyhow::Result;
use candle_core::{DType, Tensor};
use rayon::prelude::*;

use crate::modules::attention::StackedKv;
use crate::tts_model::{TTSModel, estimate_frames_after_eos, prepare_text_prompt};
use crate::voice_state::{
    ATTN_K_BUF_KEY, ATTN_V_BUF_KEY, ModelState, init_states, read_attention_cursor,
};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub struct StreamId(pub u64);

struct Stream {
    id: StreamId,
    mimi_state: ModelState,
    backbone_input: Tensor,
    step: usize,
    max_gen_len: usize,
    frames_after_eos: usize,
    eos_step: Option<usize>,
}

pub struct Frame {
    pub id: StreamId,
    /// [channels, samples] at the model's sample rate.
    pub audio: Tensor,
    pub done: bool,
}

#[derive(Debug, Clone, Copy, Default)]
pub struct StepStats {
    pub streams: usize,
    pub lm_ms: f64,
    pub mimi_ms: f64,
}

pub struct Batcher {
    model: TTSModel,
    time_embeddings: Tensor,
    /// One stacked cache per flow-LM layer; row `i` belongs to `streams[i]`.
    caches: Vec<StackedKv>,
    streams: Vec<Stream>,
    next_id: u64,
    pub last_step: StepStats,
}

impl Batcher {
    pub fn new(model: TTSModel) -> Result<Self> {
        let time_embeddings = model.flow_lm.flow_net.compute_time_embeddings(
            model.lsd_decode_steps,
            &model.device,
            DType::F32,
        )?;
        Ok(Self {
            model,
            time_embeddings,
            caches: Vec::new(),
            streams: Vec::new(),
            next_id: 0,
            last_step: StepStats::default(),
        })
    }

    pub fn model(&self) -> &TTSModel {
        &self.model
    }

    pub fn len(&self) -> usize {
        self.streams.len()
    }

    pub fn is_empty(&self) -> bool {
        self.streams.is_empty()
    }

    /// Prompts the text into a copy of the voice state and moves the result
    /// into the stacked caches; the stream then produces audio on each `step`
    /// until it is done.
    pub fn add(&mut self, text: &str, voice_state: &ModelState) -> Result<StreamId> {
        let prepared = prepare_text_prompt(text);
        let tokens = self
            .model
            .conditioner
            .prepare(&prepared, &self.model.device)?;
        let text_embeddings = self.model.conditioner.forward(&tokens)?;
        let mut state = voice_state.clone();
        let transformer = &self.model.flow_lm.transformer;
        transformer.forward(&text_embeddings, &mut state, 0)?;

        for i in 0..transformer.num_layers() {
            let name = transformer.layer_state_name(i);
            let module = state
                .get(&name)
                .ok_or_else(|| anyhow::anyhow!("prompted state lacks {name}"))?;
            let cursor = read_attention_cursor(module);
            let k = module[ATTN_K_BUF_KEY].narrow(2, 0, cursor.len)?;
            let v = module[ATTN_V_BUF_KEY].narrow(2, 0, cursor.len)?;
            if self.caches.len() <= i {
                let (_, h, _, d) = k.dims4()?;
                self.caches
                    .push(StackedKv::empty(h, d, &self.model.device)?);
            }
            self.caches[i].add_row(&k, &v, cursor.pos)?;
        }

        let id = StreamId(self.next_id);
        self.next_id += 1;
        self.streams.push(Stream {
            id,
            mimi_state: init_states(1, 1000),
            backbone_input: self
                .model
                .flow_lm
                .bos_emb
                .reshape((1, 1, self.model.ldim))?,
            step: 0,
            max_gen_len: (prepared.split_whitespace().count() + 2) * 13,
            frames_after_eos: estimate_frames_after_eos(text),
            eos_step: None,
        });
        Ok(id)
    }

    pub fn remove(&mut self, id: StreamId) -> Result<()> {
        if let Some(i) = self.streams.iter().position(|s| s.id == id) {
            self.remove_at(i)?;
        }
        Ok(())
    }

    fn remove_at(&mut self, i: usize) -> Result<()> {
        for cache in &mut self.caches {
            cache.remove_row(i)?;
        }
        self.streams.remove(i);
        Ok(())
    }

    /// Advances every stream by one frame. Streams reported `done` are
    /// dropped from the batch.
    pub fn step(&mut self) -> Result<Vec<Frame>> {
        if self.streams.is_empty() {
            return Ok(Vec::new());
        }
        let model = &self.model;
        let t_lm = std::time::Instant::now();
        let inputs: Vec<&Tensor> = self.streams.iter().map(|s| &s.backbone_input).collect();
        let sequence = Tensor::cat(&inputs, 0)?;
        let (latents, eos) = model.flow_lm.forward_stacked(
            &sequence,
            &mut self.caches,
            &self.time_embeddings,
            model.temp,
            model.eos_threshold,
        )?;
        let denorm = latents
            .broadcast_mul(&model.flow_lm.emb_std)?
            .broadcast_add(&model.flow_lm.emb_mean)?;
        let lm_ms = t_lm.elapsed().as_secs_f64() * 1000.0;

        let t_mimi = std::time::Instant::now();
        let frames: Vec<Result<Frame>> = self
            .streams
            .par_iter_mut()
            .enumerate()
            .map(|(i, stream)| {
                let latent = latents.narrow(0, i, 1)?;
                let mimi_input = denorm.narrow(0, i, 1)?.unsqueeze(1)?.transpose(1, 2)?;
                let quantized = model.mimi.quantize(&mimi_input)?;
                let audio = model
                    .mimi
                    .decode_from_latent(&quantized, &mut stream.mimi_state, stream.step)?
                    .squeeze(0)?;

                if eos[i] && stream.eos_step.is_none() {
                    stream.eos_step = Some(stream.step);
                }
                let past_eos = stream
                    .eos_step
                    .is_some_and(|e| stream.step >= e + stream.frames_after_eos);
                let done = past_eos || stream.step + 1 >= stream.max_gen_len;
                stream.backbone_input = latent.unsqueeze(1)?;
                stream.step += 1;
                Ok(Frame {
                    id: stream.id,
                    audio,
                    done,
                })
            })
            .collect();
        let frames = frames.into_iter().collect::<Result<Vec<_>>>()?;
        self.last_step = StepStats {
            streams: frames.len(),
            lm_ms,
            mimi_ms: t_mimi.elapsed().as_secs_f64() * 1000.0,
        };
        for i in (0..frames.len()).rev() {
            if frames[i].done {
                self.remove_at(i)?;
            }
        }
        Ok(frames)
    }
}
