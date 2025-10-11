mod config;
mod middleware;
mod proxy;
mod sse;
mod hitl;

use axum::{
    routing::{delete, get, post, put},
    Router,
};
use std::net::SocketAddr;
use tower_http::{
    cors::CorsLayer,
    trace::TraceLayer,
};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

use crate::{
    config::Config,
    middleware::rate_limit_middleware,
    proxy::ProxyState,
    sse::SSEManager,
};

#[tokio::main]
async fn main() {
    // Load environment variables
    dotenvy::dotenv().ok();

    // Load configuration
    let config = Config::from_env();

    // Initialize tracing
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| format!("{}=debug,tower_http=debug", env!("CARGO_PKG_NAME")).into()),
        )
        .with(tracing_subscriber::fmt::layer().json())
        .init();

    tracing::info!(
        port = config.port,
        services = ?config.services.keys().collect::<Vec<_>>(),
        "Starting API Gateway"
    );

    // Initialize shared state
    let proxy_state = ProxyState::new(config.clone());
    let sse_manager = SSEManager::new();

    // Build application router
    let app = Router::new()
        // Health check
        .route("/health", get(health_check))

        // SSE routes (special handling, not proxied)
        .route("/api/v1/events/:client_id", get(sse::handle_sse))
        .route("/api/v1/events/:client_id/response", post(hitl::handle_response))

        // Generic proxy route - forwards everything to backend services
        // Pattern: /api/v1/:service/*path
        // Examples:
        //   /api/v1/orchestrator/workflows -> http://localhost:8080/workflows
        //   /api/v1/runner/runs/123 -> http://localhost:8082/runs/123
        //   /api/v1/hitl/approvals -> http://localhost:8083/approvals
        .route("/api/v1/:service/*path",
            get(proxy::proxy_handler)
                .post(proxy::proxy_handler)
                .put(proxy::proxy_handler)
                .delete(proxy::proxy_handler)
                .patch(proxy::proxy_handler)
        )

        // Shared state
        .with_state((proxy_state, sse_manager))

        // Middleware layers (bottom to top)
        .layer(TraceLayer::new_for_http())
        .layer(CorsLayer::permissive())
        .layer(axum::middleware::from_fn(rate_limit_middleware));

    // Start server
    let addr = SocketAddr::from(([0, 0, 0, 0], config.port));
    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .expect("Failed to bind port");

    tracing::info!("Listening on {}", addr);

    axum::serve(listener, app)
        .await
        .expect("Server error");
}

async fn health_check() -> axum::Json<serde_json::Value> {
    axum::Json(serde_json::json!({
        "status": "ok",
        "service": "api-gateway",
        "version": env!("CARGO_PKG_VERSION"),
    }))
}
