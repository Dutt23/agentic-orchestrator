use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::{
        sse::{Event, KeepAlive, Sse},
        IntoResponse,
    },
};
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    convert::Infallible,
    sync::Arc,
    time::Duration,
};
use tokio::sync::{mpsc, RwLock};
use tokio_stream::{wrappers::ReceiverStream, StreamExt};
use uuid::Uuid;

use crate::proxy::ProxyState;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SSEEvent {
    pub id: String,
    pub event_type: String,
    pub timestamp: chrono::DateTime<chrono::Utc>,
    pub data: serde_json::Value,
}

#[derive(Clone)]
struct ClientChannel {
    sender: mpsc::Sender<SSEEvent>,
    response_tx: mpsc::Sender<serde_json::Value>,
}

#[derive(Clone)]
pub struct SSEManager {
    clients: Arc<RwLock<HashMap<String, ClientChannel>>>,
}

impl SSEManager {
    pub fn new() -> Self {
        let manager = Self {
            clients: Arc::new(RwLock::new(HashMap::new())),
        };

        // Spawn cleanup task
        let cleanup_clients = manager.clients.clone();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(Duration::from_secs(60));
            loop {
                interval.tick().await;
                cleanup_clients.write().await.retain(|client_id, channel| {
                    // Try to send a ping, if it fails, the client is disconnected
                    if channel.sender.is_closed() {
                        tracing::info!(client_id = %client_id, "Removing disconnected client");
                        false
                    } else {
                        true
                    }
                });
            }
        });

        manager
    }

    pub async fn register_client(&self, client_id: String) -> (mpsc::Receiver<SSEEvent>, mpsc::Receiver<serde_json::Value>) {
        let (event_tx, event_rx) = mpsc::channel(100);
        let (response_tx, response_rx) = mpsc::channel(10);

        let channel = ClientChannel {
            sender: event_tx,
            response_tx,
        };

        self.clients.write().await.insert(client_id.clone(), channel);

        tracing::info!(client_id = %client_id, "Client registered");

        (event_rx, response_rx)
    }

    pub async fn unregister_client(&self, client_id: &str) {
        self.clients.write().await.remove(client_id);
        tracing::info!(client_id = %client_id, "Client unregistered");
    }

    pub async fn send_event(&self, client_id: &str, event: SSEEvent) -> Result<(), String> {
        let clients = self.clients.read().await;
        if let Some(channel) = clients.get(client_id) {
            channel.sender.send(event).await.map_err(|e| e.to_string())?;
            Ok(())
        } else {
            Err(format!("Client not found: {}", client_id))
        }
    }

    pub async fn broadcast(&self, event: SSEEvent) {
        let clients = self.clients.read().await;
        for (client_id, channel) in clients.iter() {
            if let Err(e) = channel.sender.send(event.clone()).await {
                tracing::warn!(client_id = %client_id, error = %e, "Failed to send event");
            }
        }
    }

    pub async fn wait_for_response(&self, client_id: &str, timeout: Duration) -> Result<serde_json::Value, String> {
        let channel = {
            let clients = self.clients.read().await;
            clients.get(client_id).cloned().ok_or_else(|| format!("Client not found: {}", client_id))?
        };

        tokio::time::timeout(timeout, channel.response_tx.subscribe())
            .await
            .map_err(|_| "Response timeout".to_string())?
            .map_err(|e| e.to_string())
    }
}

pub async fn handle_sse(
    Path(client_id): Path<String>,
    State((_, sse_manager)): State<(ProxyState, SSEManager)>,
) -> Sse<impl tokio_stream::Stream<Item = Result<Event, Infallible>>> {
    tracing::info!(client_id = %client_id, "SSE connection established");

    let (mut event_rx, _response_rx) = sse_manager.register_client(client_id.clone()).await;

    // Send initial connection event
    let connection_event = SSEEvent {
        id: Uuid::new_v4().to_string(),
        event_type: "connection.established".to_string(),
        timestamp: chrono::Utc::now(),
        data: serde_json::json!({
            "client_id": client_id,
            "message": "Connected to API Gateway"
        }),
    };

    if let Err(e) = sse_manager.send_event(&client_id, connection_event).await {
        tracing::error!(error = %e, "Failed to send connection event");
    }

    let sse_manager_clone = sse_manager.clone();
    let client_id_clone = client_id.clone();

    // Create event stream
    let stream = async_stream::stream! {
        // Heartbeat interval
        let mut heartbeat = tokio::time::interval(Duration::from_secs(30));

        loop {
            tokio::select! {
                // Event from channel
                Some(event) = event_rx.recv() => {
                    let data = serde_json::to_string(&event).unwrap_or_default();
                    yield Ok(Event::default()
                        .id(event.id)
                        .event(event.event_type)
                        .data(data));
                }

                // Heartbeat
                _ = heartbeat.tick() => {
                    yield Ok(Event::default()
                        .event("heartbeat")
                        .data(serde_json::json!({
                            "timestamp": chrono::Utc::now()
                        }).to_string()));
                }
            }
        }
    };

    // Cleanup on disconnect
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_secs(1)).await;
        sse_manager_clone.unregister_client(&client_id_clone).await;
    });

    Sse::new(stream).keep_alive(KeepAlive::default())
}
