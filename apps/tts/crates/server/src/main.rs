use std::net::SocketAddr;

use axum::{Json, Router, routing::get};
use clap::Parser;

#[derive(Parser, Debug)]
struct Args {
    #[arg(long, env = "LISTEN_ADDR", default_value = "0.0.0.0:5000")]
    listen: SocketAddr,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();
    let args = Args::parse();
    let app = Router::new().route("/healthz", get(|| async { Json(serde_json::json!({"ok": true})) }));
    let listener = tokio::net::TcpListener::bind(args.listen).await?;
    tracing::info!(addr = %args.listen, "listening");
    axum::serve(listener, app).await?;
    Ok(())
}
