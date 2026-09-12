use std::time::Instant;

use tts_engine::TTSModel;
use tts_engine::batch::Batcher;
use tts_engine::weights::download_if_necessary;

const TEXT: &str = "Antonio Gramsci, né en Sardaigne en 1891, fut l'un des fondateurs du Parti \
communiste italien. Emprisonné par le régime fasciste en 1926, il rédigea en détention ses \
célèbres Cahiers de prison, où il développa la notion d'hégémonie culturelle.";

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 4 {
        anyhow::bail!("usage: batch_bench <variant> <voice> <streams> [out.wav]");
    }
    let (variant, voice, n) = (&args[1], &args[2], args[3].parse::<usize>()?);

    let mut model = TTSModel::load(variant)?;
    if std::env::var("TTS_Q8").is_ok() {
        model.quantize_batch_path()?;
    }
    let sample_rate = model.sample_rate as f64;
    let state = model.get_voice_state_from_kv_file(download_if_necessary(voice)?)?;
    let mut batcher = Batcher::new(model)?;

    let t0 = Instant::now();
    for _ in 0..n {
        batcher.add(TEXT, &state)?;
    }
    let prompt_s = t0.elapsed().as_secs_f64();

    let mut samples = vec![0usize; n];
    let mut first_ms = vec![None; n];
    let mut audio0 = Vec::new();
    let mut steps = 0usize;
    let mut step_ms = Vec::new();
    let (mut lm_ms, mut mimi_ms) = (0.0, 0.0);
    let t0 = Instant::now();
    while !batcher.is_empty() {
        let ts = Instant::now();
        let frames = batcher.step()?;
        step_ms.push(ts.elapsed().as_secs_f64() * 1000.0);
        lm_ms += batcher.last_step.lm_ms;
        mimi_ms += batcher.last_step.mimi_ms;
        steps += 1;
        for f in frames {
            let i = f.id.0 as usize;
            samples[i] += f.audio.dim(1)?;
            if first_ms[i].is_none() {
                first_ms[i] = Some(t0.elapsed().as_secs_f64() * 1000.0);
            }
            if i == 0 {
                audio0.push(f.audio);
            }
        }
    }
    let wall = t0.elapsed().as_secs_f64();
    let audio_total: f64 = samples.iter().map(|&s| s as f64 / sample_rate).sum();
    let audio_min = samples
        .iter()
        .map(|&s| s as f64 / sample_rate)
        .fold(f64::MAX, f64::min);
    step_ms.sort_by(|a, b| a.partial_cmp(b).unwrap());
    let p50 = step_ms[step_ms.len() / 2];
    let p95 = step_ms[step_ms.len() * 95 / 100];

    println!(
        "streams={n} prompt_s={prompt_s:.2} steps={steps} wall_s={wall:.2} step_ms_p50={p50:.1} \
         step_ms_p95={p95:.1} audio_total_s={audio_total:.1} rtf_per_stream={:.3} \
         first_audio_ms={:.0} lm_ms_per_step={:.1} mimi_ms_per_step={:.1}",
        wall / audio_min,
        first_ms[0].unwrap_or(0.0),
        lm_ms / steps as f64,
        mimi_ms / steps as f64
    );

    if let Some(out) = args.get(4) {
        let audio = candle_core::Tensor::cat(&audio0, 1)?;
        tts_engine::audio::write_wav(out, &audio, sample_rate as u32)?;
    }
    Ok(())
}
