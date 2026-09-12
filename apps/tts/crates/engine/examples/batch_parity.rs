//! With the sampling temperature at zero the generation is deterministic, so
//! one text must come out identical whether it is read alone through the
//! reference path, alone through the batcher, or next to other streams.

use candle_core::Tensor;
use tts_engine::TTSModel;
use tts_engine::batch::Batcher;
use tts_engine::voice_state::ModelState;
use tts_engine::weights::download_if_necessary;

const A: &str =
    "Bonjour, ceci est une lecture faite par le nouveau serveur de parole, en français.";
const B: &str = "Antonio Gramsci, né en Sardaigne en 1891, fut l'un des fondateurs du Parti communiste italien.";
const C: &str = "Il pleut.";

fn batched(model: &TTSModel, voice: &ModelState, texts: &[&str]) -> anyhow::Result<Vec<Tensor>> {
    let mut batcher = Batcher::new(model.clone())?;
    let mut ids = Vec::new();
    for t in texts {
        ids.push(batcher.add(t, voice)?);
    }
    let mut chunks: Vec<Vec<Tensor>> = vec![Vec::new(); texts.len()];
    while !batcher.is_empty() {
        for f in batcher.step()? {
            let i = ids.iter().position(|id| *id == f.id).unwrap();
            chunks[i].push(f.audio);
        }
    }
    chunks
        .into_iter()
        .map(|c| Ok(Tensor::cat(&c, 1)?))
        .collect()
}

fn reference(model: &TTSModel, voice: &ModelState, text: &str) -> anyhow::Result<Tensor> {
    let chunks: Vec<Tensor> = model
        .generate_stream_long(text, voice)
        .collect::<Result<_, _>>()?;
    let last = chunks[0].rank() - 1;
    let audio = Tensor::cat(&chunks, last)?;
    Ok(if audio.rank() == 3 {
        audio.squeeze(0)?
    } else {
        audio
    })
}

fn compare(name: &str, x: &Tensor, y: &Tensor) -> anyhow::Result<()> {
    let n = x.dim(1)?.min(y.dim(1)?);
    let xs = x.narrow(1, 0, n)?.flatten_all()?.to_vec1::<f32>()?;
    let ys = y.narrow(1, 0, n)?.flatten_all()?.to_vec1::<f32>()?;
    let max = xs
        .iter()
        .zip(&ys)
        .map(|(a, b)| (a - b).abs())
        .fold(0f32, f32::max);
    println!(
        "{name}: samples {} vs {}, max abs diff {max:.5}",
        x.dim(1)?,
        y.dim(1)?
    );
    let frame = 1920;
    let per: Vec<String> = (0..(n / frame).min(24))
        .map(|f| {
            let m = xs[f * frame..(f + 1) * frame]
                .iter()
                .zip(&ys[f * frame..(f + 1) * frame])
                .map(|(a, b)| (a - b).abs())
                .fold(0f32, f32::max);
            format!("{m:.1e}")
        })
        .collect();
    println!("  per-frame max diff: {}", per.join(" "));
    Ok(())
}

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    let (variant, voice) = (&args[1], &args[2]);
    let mut model = TTSModel::load(variant)?;
    model.temp = 0.0;
    if std::env::var("TTS_Q8").is_ok() {
        model.quantize_batch_path()?;
    }
    let state = model.get_voice_state_from_kv_file(download_if_necessary(voice)?)?;

    let reference_a = reference(&model, &state, A)?;
    let reference_a2 = reference(&model, &state, A)?;
    compare("reference vs reference", &reference_a, &reference_a2)?;
    let alone = batched(&model, &state, &[A])?;
    let alone2 = batched(&model, &state, &[A])?;
    compare("batcher alone vs batcher alone", &alone[0], &alone2[0])?;
    let with_b = batched(&model, &state, &[A, B])?;
    let with_bc = batched(&model, &state, &[C, A, B])?;

    compare("batcher alone vs reference", &alone[0], &reference_a)?;
    compare("batched with B vs alone", &with_b[0], &alone[0])?;
    compare("batched with C and B vs alone", &with_bc[1], &alone[0])?;
    let out = args.get(3).cloned().unwrap_or_default();
    if !out.is_empty() {
        tts_engine::audio::write_wav(
            format!("{out}_alone.wav"),
            &alone[0],
            model.sample_rate as u32,
        )?;
        tts_engine::audio::write_wav(
            format!("{out}_with_b.wav"),
            &with_b[0],
            model.sample_rate as u32,
        )?;
    }
    Ok(())
}
