use std::sync::Mutex;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};

const STEP_BUCKETS_MS: [f64; 10] = [
    10.0, 20.0, 40.0, 60.0, 80.0, 100.0, 150.0, 200.0, 400.0, 800.0,
];
const FIRST_AUDIO_BUCKETS_MS: [f64; 9] = [
    50.0, 100.0, 200.0, 300.0, 500.0, 1000.0, 2000.0, 4000.0, 8000.0,
];

pub struct Histogram {
    bounds: &'static [f64],
    counts: Vec<AtomicU64>,
    sum: Mutex<f64>,
    total: AtomicU64,
}

impl Histogram {
    fn new(bounds: &'static [f64]) -> Self {
        Self {
            bounds,
            counts: bounds.iter().map(|_| AtomicU64::new(0)).collect(),
            sum: Mutex::new(0.0),
            total: AtomicU64::new(0),
        }
    }

    pub fn observe(&self, v: f64) {
        for (bound, count) in self.bounds.iter().zip(&self.counts) {
            if v <= *bound {
                count.fetch_add(1, Ordering::Relaxed);
            }
        }
        self.total.fetch_add(1, Ordering::Relaxed);
        *self.sum.lock().unwrap() += v;
    }

    fn render(&self, out: &mut String, name: &str) {
        for (bound, count) in self.bounds.iter().zip(&self.counts) {
            out.push_str(&format!(
                "{name}_bucket{{le=\"{bound}\"}} {}\n",
                count.load(Ordering::Relaxed)
            ));
        }
        let total = self.total.load(Ordering::Relaxed);
        out.push_str(&format!("{name}_bucket{{le=\"+Inf\"}} {total}\n"));
        out.push_str(&format!("{name}_sum {}\n", *self.sum.lock().unwrap()));
        out.push_str(&format!("{name}_count {total}\n"));
    }
}

/// What the scheduler exposes: slot occupancy for admission, and the two
/// latencies that decide whether listeners hear gaps.
pub struct Metrics {
    pub live_active: AtomicUsize,
    pub live_queued: AtomicUsize,
    pub background_active: AtomicUsize,
    pub background_queued: AtomicUsize,
    pub admitted: AtomicU64,
    pub rejected: AtomicU64,
    pub step_ms: Histogram,
    pub first_audio_ms: Histogram,
    pub audio_seconds: Mutex<f64>,
}

impl Metrics {
    pub fn new() -> Self {
        Self {
            live_active: AtomicUsize::new(0),
            live_queued: AtomicUsize::new(0),
            background_active: AtomicUsize::new(0),
            background_queued: AtomicUsize::new(0),
            admitted: AtomicU64::new(0),
            rejected: AtomicU64::new(0),
            step_ms: Histogram::new(&STEP_BUCKETS_MS),
            first_audio_ms: Histogram::new(&FIRST_AUDIO_BUCKETS_MS),
            audio_seconds: Mutex::new(0.0),
        }
    }

    pub fn render(&self) -> String {
        let mut out = String::new();
        let gauge = |out: &mut String, name: &str, v: usize| {
            out.push_str(&format!("# TYPE {name} gauge\n{name} {v}\n"));
        };
        gauge(
            &mut out,
            "tts_live_active",
            self.live_active.load(Ordering::Relaxed),
        );
        gauge(
            &mut out,
            "tts_live_queued",
            self.live_queued.load(Ordering::Relaxed),
        );
        gauge(
            &mut out,
            "tts_background_active",
            self.background_active.load(Ordering::Relaxed),
        );
        gauge(
            &mut out,
            "tts_background_queued",
            self.background_queued.load(Ordering::Relaxed),
        );
        out.push_str(&format!(
            "# TYPE tts_requests_admitted_total counter\ntts_requests_admitted_total {}\n",
            self.admitted.load(Ordering::Relaxed)
        ));
        out.push_str(&format!(
            "# TYPE tts_requests_rejected_total counter\ntts_requests_rejected_total {}\n",
            self.rejected.load(Ordering::Relaxed)
        ));
        out.push_str(&format!(
            "# TYPE tts_audio_seconds_total counter\ntts_audio_seconds_total {}\n",
            *self.audio_seconds.lock().unwrap()
        ));
        out.push_str("# TYPE tts_step_ms histogram\n");
        self.step_ms.render(&mut out, "tts_step_ms");
        out.push_str("# TYPE tts_first_audio_ms histogram\n");
        self.first_audio_ms.render(&mut out, "tts_first_audio_ms");
        out
    }
}
