use axum::{
    extract::{Path, State},
    http::StatusCode,
    Json,
};
use serde::{Deserialize, Serialize};

use crate::{proxy::ProxyState, sse::SSEManager};

#[derive(Debug, Serialize, Deserialize)]
pub struct Response {
    pub question_id: String,
    pub answer: serde_json::Value,
}

#[derive(Debug, Serialize)]
pub struct ResponseAck {
    pub status: String,
    pub message: String,
}

pub async fn handle_response(
    Path(client_id): Path<String>,
    State((_, sse_manager)): State<(ProxyState, SSEManager)>,
    Json(response): Json<Response>,
) -> Result<Json<ResponseAck>, StatusCode> {
    tracing::info!(
        client_id = %client_id,
        question_id = %response.question_id,
        "Received client response"
    );

    // Get client channel
    let clients = sse_manager.clients.read().await;
    let channel = clients.get(&client_id).ok_or_else(|| {
        tracing::error!(client_id = %client_id, "Client not found");
        StatusCode::NOT_FOUND
    })?;

    // Send response to response channel
    let response_data = serde_json::json!({
        "question_id": response.question_id,
        "answer": response.answer,
    });

    channel
        .response_tx
        .send(response_data)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "Failed to send response");
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

    Ok(Json(ResponseAck {
        status: "ok".to_string(),
        message: "Response received".to_string(),
    }))
}
