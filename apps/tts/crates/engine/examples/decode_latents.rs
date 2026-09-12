use candle_core::{Device, Tensor};
use tts_engine::TTSModel;
use tts_engine::voice_state::init_states;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    if args.len() < 4 {
        anyhow::bail!("usage: decode_latents <variant> <latents.safetensors> <out.wav>");
    }
    let model = TTSModel::load(&args[1])?;
    let tensors = candle_core::safetensors::load(&args[2], &Device::Cpu)?;
    let latents = tensors["latents"].clone(); // [1, T, C], already denormalised
    let (_, t, _) = latents.dims3()?;
    let mut state = init_states(1, 1000);
    let mut frames = Vec::new();
    for step in 0..t {
        let frame = latents.narrow(1, step, 1)?; // [1, 1, C]
        let quantized = model.mimi.quantize(&frame.transpose(1, 2)?)?;
        let audio = model
            .mimi
            .decode_from_latent(&quantized, &mut state, step)?;
        frames.push(audio.squeeze(0)?);
    }
    let audio = Tensor::cat(&frames, 1)?;
    tts_engine::audio::write_wav(&args[3], &audio, model.sample_rate as u32)?;
    println!("decoded {} frames, {} samples", t, audio.dim(1)?);
    Ok(())
}
