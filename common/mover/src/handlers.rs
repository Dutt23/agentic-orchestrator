/// Operation handlers for mover service
///
/// Each handler processes a specific OpCode and returns a MoverResponse

use crate::protocol::{MoverRequest, MoverResponse};
use std::sync::Arc;
use tracing::error;

// PostgresManager placeholder since we disabled it
pub struct PostgresManager;

/// Handle READ operation - Query Postgres via io_uring (disabled for glommio)
///
/// NOTE: This COULD use splice too! Here's how:
/// 1. Get cas_id from req.id
/// 2. Connect to Postgres (postgres:5432)
/// 3. Send: "SELECT content FROM cas_blob WHERE cas_id = 'xxx'"
/// 4. splice(postgres_socket → unix_socket)  ← ZERO-COPY!
///
/// Same relay_bidirectional() primitive as HTTP!
/// Just forwarding Postgres wire protocol bytes instead of HTTP bytes.
pub async fn handle_read(_postgres: &Option<Arc<PostgresManager>>, _req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("Postgres not yet supported with glommio".to_string())
}

/// Handle WRITE operation - Write to Postgres via io_uring (disabled for glommio)
pub async fn handle_write(_postgres: &Option<Arc<PostgresManager>>, _req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("Postgres not yet supported with glommio".to_string())
}

/// Handle SEND_ZC operation (placeholder)
pub async fn handle_send_zc(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("SEND_ZC not implemented".to_string())
}

/// Handle RECV operation (placeholder)
pub async fn handle_recv(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("RECV not implemented".to_string())
}

/// Handle BATCH operation (placeholder)
pub async fn handle_batch(_req: &MoverRequest) -> MoverResponse {
    MoverResponse::error("BATCH not implemented".to_string())
}

/// Handle HTTP proxy operation - Proxy HTTP requests via io_uring
pub async fn handle_http(req: &MoverRequest) -> MoverResponse {
    use crate::monoio_http;
    use serde::{Deserialize, Serialize};
    use std::collections::HashMap;

    #[derive(Deserialize)]
    struct HttpProxyRequest {
        method: String,
        url: String,
        headers: HashMap<String, String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        body: Option<Vec<u8>>,
    }

    #[derive(Serialize)]
    struct HttpProxyResponse {
        status_code: u16,
        headers: HashMap<String, String>,
        body: Vec<u8>,
    }

    // Deserialize the HTTP request from JSON
    let http_req: HttpProxyRequest = match serde_json::from_slice(&req.data) {
        Ok(r) => r,
        Err(e) => {
            error!("Failed to parse HTTP request: {}", e);
            return MoverResponse::error(format!("Invalid HTTP request: {}", e));
        }
    };

    // Execute HTTP request using monoio (PURE io_uring with true zero-copy!)
    // Monoio exposes raw FDs for splice and send_zc operations
    let result = monoio_http::IoUringHttpClient::request(
        &http_req.method,
        &http_req.url,
        http_req.headers,
        http_req.body,
    )
    .await;

    match result {
        Ok((status_code, headers, body)) => {
            let http_resp = HttpProxyResponse {
                status_code,
                headers,
                body,
            };

            match serde_json::to_vec(&http_resp) {
                Ok(data) => MoverResponse::ok(data),
                Err(e) => {
                    error!("Failed to serialize response: {}", e);
                    MoverResponse::error(format!("Serialization failed: {}", e))
                }
            }
        }
        Err(e) => {
            error!("HTTP proxy error (glommio): {}", e);
            MoverResponse::error(e)
        }
    }
}
