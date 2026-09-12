use candle_core::Tensor;
use tts_engine::TTSModel;
use tts_engine::weights::download_if_necessary;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    let model = TTSModel::load(&args[1])?;
    let state = model.get_voice_state_from_kv_file(download_if_necessary(&args[2])?)?;
    let n: usize = args[4].parse()?;
    for i in 0..n {
        let chunks: Vec<Tensor> = model
            .generate_stream(&args[3], &state)
            .collect::<Result<_, _>>()?;
        let last = chunks[0].rank() - 1;
        let audio = Tensor::cat(&chunks, last)?;
        let audio = if audio.rank() == 3 {
            audio.squeeze(0)?
        } else {
            audio
        };
        let samples = audio.dim(1)?;
        let sr = model.sample_rate as f64;
        let win = (0.25 * sr) as usize;
        let v = audio.flatten_all()?.to_vec1::<f32>()?;
        let prof: Vec<String> = (0..(samples / win).min(16))
            .map(|f| {
                let x = &v[f * win..(f + 1) * win];
                let rms = (x.iter().map(|s| s * s).sum::<f32>() / win as f32).sqrt();
                format!("{:4.0}", 20.0 * (rms + 1e-9).log10())
            })
            .collect();
        println!("run {i}: {:.2}s  {}", samples as f64 / sr, prof.join(" "));
    }
    Ok(())
}
