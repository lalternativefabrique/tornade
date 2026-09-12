mod encode;
mod engine;
mod metrics;

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Instant;

use axum::body::Body;
use axum::extract::State;
use axum::http::{HeaderMap, StatusCode, header};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::{Json, Router};
use clap::Parser;
use serde::Deserialize;
use tts_engine::TTSModel;
use tts_engine::voice_state::ModelState;

use crate::encode::Format;
use crate::engine::{Class, EngineHandle, Job};
use crate::metrics::Metrics;

#[derive(Parser, Debug)]
struct Args {
    #[arg(long, env = "LISTEN_ADDR", default_value = "0.0.0.0:5000")]
    listen: SocketAddr,
    /// Model config name under the engine's config directory.
    #[arg(long, env = "TTS_MODEL", default_value = "french_24l")]
    model: String,
    /// `name=path` pairs; a path is a local file or an hf:// URL to a voice
    /// state. The first one is the default voice.
    #[arg(
        long,
        env = "TTS_VOICES",
        value_delimiter = ',',
        default_value = "estelle=hf://kyutai/pocket-tts-without-voice-cloning/languages/french_24l/embeddings/estelle.safetensors@e81d79e8194ad4c7ce879c87a4258ef20cbf2487"
    )]
    voices: Vec<String>,
    /// Readings a listener is waiting on, served at once or refused.
    #[arg(long, env = "TTS_LIVE_SLOTS", default_value_t = 8)]
    live_slots: usize,
    /// Readings nobody waits on; they only take slots the live ones leave.
    #[arg(long, env = "TTS_BACKGROUND_SLOTS", default_value_t = 2)]
    background_slots: usize,
    #[arg(long, env = "TTS_Q8", default_value_t = true, action = clap::ArgAction::Set)]
    q8: bool,
    /// Output gain in dB applied to every reading.
    #[arg(long, env = "TTS_GAIN_DB", default_value_t = 10.0)]
    gain_db: f32,
}

struct AppState {
    engine: EngineHandle,
    voices: HashMap<String, Arc<ModelState>>,
    default_voice: String,
    model: Arc<TTSModel>,
}

#[derive(Deserialize)]
struct SpeechRequest {
    input: String,
    #[serde(default)]
    voice: Option<String>,
    #[serde(default)]
    response_format: Option<String>,
    #[serde(default)]
    model: Option<String>,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| "info".into()),
        )
        .init();
    let args = Args::parse();

    let t0 = Instant::now();
    let mut model = TTSModel::load(&args.model)?;
    if args.q8 {
        model.quantize_batch_path()?;
    }
    let mut voices = HashMap::new();
    let mut default_voice = None;
    for spec in &args.voices {
        let (name, path) = spec
            .split_once('=')
            .ok_or_else(|| anyhow::anyhow!("voice {spec:?} is not name=path"))?;
        let file = tts_engine::weights::download_if_necessary(path)?;
        let state = model.get_voice_state_from_kv_file(&file)?;
        voices.insert(name.to_string(), Arc::new(state));
        default_voice.get_or_insert_with(|| name.to_string());
    }
    let default_voice = default_voice.ok_or_else(|| anyhow::anyhow!("no voice configured"))?;
    tracing::info!(
        model = %args.model,
        q8 = args.q8,
        voices = ?voices.keys().collect::<Vec<_>>(),
        load_s = t0.elapsed().as_secs_f64(),
        "model ready"
    );

    let metrics = Arc::new(Metrics::new());
    let model = Arc::new(model);
    let engine = engine::spawn(
        (*model).clone(),
        args.live_slots,
        args.background_slots,
        encode::db_to_gain(args.gain_db),
        metrics,
    )?;
    let state = Arc::new(AppState {
        engine,
        voices,
        default_voice,
        model,
    });

    let app = Router::new()
        .route(
            "/healthz",
            get(|| async { Json(serde_json::json!({"ok": true})) }),
        )
        .route("/metrics", get(metrics_handler))
        .route("/v1/audio/speech", post(speech))
        .with_state(state);
    let listener = tokio::net::TcpListener::bind(args.listen).await?;
    tracing::info!(addr = %args.listen, "listening");
    axum::serve(listener, app).await?;
    Ok(())
}

async fn metrics_handler(State(state): State<Arc<AppState>>) -> Response {
    (
        [(header::CONTENT_TYPE, "text/plain; version=0.0.4")],
        state.engine.metrics.render(),
    )
        .into_response()
}

fn error(status: StatusCode, message: impl Into<String>) -> Response {
    (status, Json(serde_json::json!({"error": message.into()}))).into_response()
}

async fn speech(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(req): Json<SpeechRequest>,
) -> Response {
    let text = req.input.trim();
    if text.is_empty() {
        return error(StatusCode::BAD_REQUEST, "input is required");
    }
    let Some(format) = Format::parse(req.response_format.as_deref().unwrap_or("mp3")) else {
        return error(
            StatusCode::BAD_REQUEST,
            "response_format must be mp3, wav or pcm",
        );
    };
    let voice_name = req.voice.as_deref().unwrap_or(&state.default_voice);
    let Some(voice) = state.voices.get(voice_name).or_else(|| {
        tracing::warn!(voice = voice_name, "unknown voice, using the default");
        state.voices.get(&state.default_voice)
    }) else {
        return error(StatusCode::BAD_REQUEST, "unknown voice");
    };
    let class = match headers.get("x-tts-priority").and_then(|v| v.to_str().ok()) {
        Some("background") => Class::Background,
        _ => Class::Live,
    };
    let _ = req.model;

    let segments = state.model.split_into_best_sentences(text);
    if segments.is_empty() {
        return error(StatusCode::BAD_REQUEST, "nothing to read");
    }
    let (tx, mut rx) = tokio::sync::mpsc::unbounded_channel();
    let job = Job {
        segments,
        voice: voice.clone(),
        class,
        frames: tx,
        submitted: Instant::now(),
    };
    if state.engine.submit(job).is_err() {
        return (
            StatusCode::SERVICE_UNAVAILABLE,
            [(header::RETRY_AFTER, "2")],
            Json(serde_json::json!({"error": "every slot is busy"})),
        )
            .into_response();
    }

    let mut pcm: Vec<i16> = Vec::new();
    while let Some(frame) = rx.recv().await {
        pcm.extend_from_slice(&frame);
    }
    if pcm.is_empty() {
        return error(
            StatusCode::INTERNAL_SERVER_ERROR,
            "the reading produced no audio",
        );
    }
    let sample_rate = state.engine.sample_rate;
    let encoded =
        tokio::task::spawn_blocking(move || encode::encode(format, &pcm, sample_rate)).await;
    match encoded {
        Ok(Ok(bytes)) => (
            StatusCode::OK,
            [(header::CONTENT_TYPE, format.mime())],
            Body::from(bytes),
        )
            .into_response(),
        Ok(Err(e)) => error(StatusCode::INTERNAL_SERVER_ERROR, format!("encode: {e}")),
        Err(e) => error(
            StatusCode::INTERNAL_SERVER_ERROR,
            format!("encode task: {e}"),
        ),
    }
}
