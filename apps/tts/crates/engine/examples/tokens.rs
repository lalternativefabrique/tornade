use tts_engine::TTSModel;
use tts_engine::tts_model::prepare_text_prompt;

fn main() -> anyhow::Result<()> {
    let args: Vec<String> = std::env::args().collect();
    let model = TTSModel::load(&args[1])?;
    let prepared = prepare_text_prompt(&args[2]);
    println!("rust prepared: {prepared:?}");
    let ids = model.conditioner.prepare(&prepared, &model.device)?;
    println!(
        "rust ids: {:?} {:?}",
        ids.dims(),
        ids.flatten_all()?.to_vec1::<i64>().or_else(|_| ids
            .flatten_all()?
            .to_vec1::<u32>()
            .map(|v| v.into_iter().map(|x| x as i64).collect()))?
    );
    for seg in model.split_into_best_sentences(&args[2]) {
        println!("segment: {seg:?}");
    }
    Ok(())
}
