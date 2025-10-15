use anyhow::Result;
use reqwest::Client;
use serde::{Deserialize, Serialize};
use std::env;

#[derive(Debug, Serialize)]
struct ApprovalRequest {
    run_id: String,
    node_id: String,
    approved: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    comment: Option<String>,
}

#[derive(Debug, Deserialize)]
struct ApprovalResponse {
    success: bool,
    message: String,
}

pub async fn approve(
    run_id: &str,
    node_id: &str,
    comment: Option<&str>,
    api_url: &str,
) -> Result<()> {
    let username = env::var("USER").unwrap_or_else(|_| "unknown".to_string());

    let request = ApprovalRequest {
        run_id: run_id.to_string(),
        node_id: node_id.to_string(),
        approved: true,
        comment: comment.map(|s| s.to_string()),
    };

    let client = Client::new();
    // Fanout service runs on port 8084
    let fanout_url = api_url.replace(":8081", ":8084");
    let url = format!("{}/api/approval", fanout_url);

    let response = client
        .post(&url)
        .header("Content-Type", "application/json")
        .header("X-User-ID", &username)
        .json(&request)
        .send()
        .await?;

    if response.status().is_success() {
        let result: ApprovalResponse = response.json().await?;
        println!("✓ Approval granted");
        println!("  Run: {}", run_id);
        println!("  Node: {}", node_id);
        if let Some(c) = comment {
            println!("  Comment: {}", c);
        }
        println!("  {}", result.message);
    } else {
        let status = response.status();
        let error_text = response.text().await?;
        anyhow::bail!("Approval failed ({}): {}", status, error_text);
    }

    Ok(())
}

pub async fn reject(
    run_id: &str,
    node_id: &str,
    comment: &str,
    api_url: &str,
) -> Result<()> {
    let username = env::var("USER").unwrap_or_else(|_| "unknown".to_string());

    let request = ApprovalRequest {
        run_id: run_id.to_string(),
        node_id: node_id.to_string(),
        approved: false,
        comment: Some(comment.to_string()),
    };

    let client = Client::new();
    // Fanout service runs on port 8084
    let fanout_url = api_url.replace(":8081", ":8084");
    let url = format!("{}/api/approval", fanout_url);

    let response = client
        .post(&url)
        .header("Content-Type", "application/json")
        .header("X-User-ID", &username)
        .json(&request)
        .send()
        .await?;

    if response.status().is_success() {
        let result: ApprovalResponse = response.json().await?;
        println!("✓ Approval rejected");
        println!("  Run: {}", run_id);
        println!("  Node: {}", node_id);
        println!("  Reason: {}", comment);
        println!("  {}", result.message);
    } else {
        let status = response.status();
        let error_text = response.text().await?;
        anyhow::bail!("Rejection failed ({}): {}", status, error_text);
    }

    Ok(())
}
