use anyhow::Result;
use colored::Colorize;
use serde::Serialize;

use crate::client::ApiClient;
use crate::utils::spinner;
use crate::OutputFormat;

#[derive(Serialize)]
struct ApprovalRequest {
    decision: String,
    reason: Option<String>,
}

pub async fn handle(
    client: ApiClient,
    ticket_id: String,
    approve: bool,
    reason: Option<String>,
    _output: &OutputFormat,
) -> Result<()> {
    let decision = if approve { "approve" } else { "reject" };

    let _spinner = spinner::new(&format!("{}ing approval request...", decision));

    let request = ApprovalRequest {
        decision: decision.to_string(),
        reason,
    };

    client
        .post::<_, ()>(&format!("/api/approvals/{}/decision", ticket_id), &request)
        .await?;

    drop(_spinner);

    println!(
        "{} Approval {} for ticket: {}",
        "✓".green(),
        if approve {
            "granted".green()
        } else {
            "rejected".red()
        },
        ticket_id.cyan()
    );

    Ok(())
}
