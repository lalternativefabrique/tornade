use std::time::Instant;

use candle_core::Tensor;
use tts_engine::TTSModel;
use tts_engine::weights::download_if_necessary;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 5 {
        anyhow::bail!("usage: speak <variant> <voice.safetensors|hf://…> <text> <out.wav>");
    }
    let (variant, voice, text, out) = (&args[1], &args[2], &args[3], &args[4]);

    let t0 = Instant::now();
    let model = TTSModel::load(variant)?;
    let voice_file = download_if_necessary(voice)?;
    let state = model.get_voice_state_from_kv_file(&voice_file)?;
    eprintln!("load_s={:.2}", t0.elapsed().as_secs_f64());

    let t0 = Instant::now();
    let mut first = None;
    let mut chunks = Vec::new();
    for chunk in model.generate_stream_long(text, &state) {
        let chunk = chunk?;
        if first.is_none() {
            first = Some(t0.elapsed().as_secs_f64());
        }
        chunks.push(chunk);
    }
    let gen_s = t0.elapsed().as_secs_f64();
    let last = chunks[0].rank() - 1;
    let audio = Tensor::cat(&chunks, last)?;
    let audio = if audio.rank() == 3 {
        audio.squeeze(0)?
    } else {
        audio
    };
    let audio_s = audio.dim(1)? as f64 / model.sample_rate as f64;
    tts_engine::audio::write_wav(out, &audio, model.sample_rate as u32)?;
    println!(
        "first_audio_ms={:.0} gen_s={:.2} audio_s={:.2} rtf={:.3}",
        first.unwrap_or(0.0) * 1000.0,
        gen_s,
        audio_s,
        gen_s / audio_s
    );
    Ok(())
}
