use axum::{
    body::Body,
    extract::{Path, Request, State},
    http::{StatusCode, Uri},
    response::{IntoResponse, Response},
};
use hyper::body::Incoming;
use hyper_util::{
    client::legacy::{connect::HttpConnector, Client},
    rt::TokioExecutor,
};
use std::time::Duration;

use crate::{config::Config, sse::SSEManager};

#[derive(Clone)]
pub struct ProxyState {
    client: Client<HttpConnector, Body>,
    config: Config,
}

impl ProxyState {
    pub fn new(config: Config) -> Self {
        // Create HTTP client with connection pooling
        let mut connector = HttpConnector::new();
        connector.set_keepalive(Some(Duration::from_secs(90)));
        connector.set_nodelay(true);

        let client = Client::builder(TokioExecutor::new())
            .pool_idle_timeout(Duration::from_secs(90))
            .pool_max_idle_per_host(100)
            .build(connector);

        Self { client, config }
    }

    async fn forward(&self, mut req: Request, target_url: &str, service_path: &str) -> Result<Response, StatusCode> {
        let query = req.uri().query().unwrap_or("");

        // Build target URI - forward the remaining path after service name
        let target_uri = if query.is_empty() {
            format!("{}{}", target_url, service_path)
        } else {
            format!("{}{}?{}", target_url, service_path, query)
        };

        let uri: Uri = target_uri.parse().map_err(|e| {
            tracing::error!("Invalid URI: {}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

        // Update request URI
        *req.uri_mut() = uri;

        // Add X-Forwarded headers
        let headers = req.headers_mut();
        headers.insert("X-Forwarded-Host", req.uri().host().unwrap_or("unknown").parse().unwrap());

        let start = std::time::Instant::now();

        // Forward request
        let response = self.client
            .request(req)
            .await
            .map_err(|e| {
                tracing::error!("Proxy error: {}", e);
                StatusCode::BAD_GATEWAY
            })?;

        let duration = start.elapsed();

        tracing::info!(
            target_url = %target_url,
            path = %service_path,
            status = response.status().as_u16(),
            duration_ms = duration.as_millis(),
            "Proxied request"
        );

        Ok(response.into_response())
    }
}

// Generic proxy handler - routes based on service name
pub async fn proxy_handler(
    Path((service, path)): Path<(String, String)>,
    State((proxy_state, _)): State<(ProxyState, SSEManager)>,
    req: Request,
) -> Result<Response, StatusCode> {
    // Look up service URL
    let target_url = proxy_state.config.get_service_url(&service).ok_or_else(|| {
        tracing::error!(service = %service, "Unknown service");
        StatusCode::NOT_FOUND
    })?;

    // Ensure path starts with /
    let service_path = if path.starts_with('/') {
        path
    } else {
        format!("/{}", path)
    };

    proxy_state.forward(req, target_url, &service_path).await
}
