use anyhow::{Context, Result};
use colored::Colorize;
use serde::{Deserialize, Serialize};
use std::fs;

use crate::client::ApiClient;
use crate::commands::{logs, RunCommands};
use crate::utils::spinner;
use crate::OutputFormat;

#[derive(Serialize)]
struct StartRunRequest {
    workflow: serde_json::Value,
    inputs: Option<serde_json::Value>,
}

#[derive(Deserialize)]
struct RunResponse {
    run_id: String,
    status: String,
}

#[derive(Deserialize)]
struct RunStatus {
    run_id: String,
    status: String,
    started_at: String,
    ended_at: Option<String>,
    active_nodes: Vec<String>,
}

#[derive(Deserialize)]
struct RunListResponse {
    runs: Vec<RunStatus>,
    total: usize,
}

pub async fn handle(
    client: ApiClient,
    command: RunCommands,
    output: &OutputFormat,
) -> Result<()> {
    match command {
        RunCommands::Start {
            workflow,
            inputs,
            follow,
        } => start_run(client, workflow, inputs, follow, output).await,
        RunCommands::Status { run_id } => get_status(client, run_id, output).await,
        RunCommands::List { status, limit } => list_runs(client, status, limit, output).await,
        RunCommands::Cancel { run_id } => cancel_run(client, run_id, output).await,
    }
}

async fn start_run(
    client: ApiClient,
    workflow_path: String,
    inputs_path: Option<String>,
    follow: bool,
    output: &OutputFormat,
) -> Result<()> {
    // Load workflow
    let workflow_content = fs::read_to_string(&workflow_path)
        .with_context(|| format!("Failed to read workflow file: {}", workflow_path))?;

    let workflow: serde_json::Value = serde_json::from_str(&workflow_content)
        .context("Invalid workflow JSON")?;

    // Load inputs if provided
    let inputs = if let Some(inputs_path) = inputs_path {
        let inputs_content = fs::read_to_string(&inputs_path)
            .with_context(|| format!("Failed to read inputs file: {}", inputs_path))?;
        Some(serde_json::from_str(&inputs_content).context("Invalid inputs JSON")?)
    } else {
        None
    };

    // Start run with spinner
    let _spinner = spinner::new("Starting workflow run...");

    let request = StartRunRequest { workflow, inputs };
    let response: RunResponse = client.post("/api/runs", &request).await?;

    drop(_spinner);

    match output {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&response)?);
        }
        _ => {
            println!("{} Run started: {}", "✓".green(), response.run_id.cyan());
            println!("Status: {}", response.status.yellow());
        }
    }

    // Follow logs if requested
    if follow {
        println!();
        logs::stream_logs(client, response.run_id, None, "all".to_string()).await?;
    }

    Ok(())
}

async fn get_status(client: ApiClient, run_id: String, output: &OutputFormat) -> Result<()> {
    let status: RunStatus = client.get(&format!("/api/runs/{}", run_id)).await?;

    match output {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&status)?);
        }
        _ => {
            println!("{} Run: {}", "•".blue(), status.run_id.cyan());
            println!("  Status: {}", format_status(&status.status));
            println!("  Started: {}", status.started_at);
            if let Some(ended) = status.ended_at {
                println!("  Ended: {}", ended);
            }
            if !status.active_nodes.is_empty() {
                println!("  Active nodes:");
                for node in status.active_nodes {
                    println!("    - {}", node.yellow());
                }
            }
        }
    }

    Ok(())
}

async fn list_runs(
    client: ApiClient,
    status: Option<String>,
    limit: usize,
    output: &OutputFormat,
) -> Result<()> {
    let mut path = format!("/api/runs?limit={}", limit);
    if let Some(status) = status {
        path.push_str(&format!("&status={}", status));
    }

    let response: RunListResponse = client.get(&path).await?;

    match output {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&response)?);
        }
        _ => {
            println!("Runs (showing {} of {}):", response.runs.len(), response.total);
            println!();
            for run in response.runs {
                println!(
                    "{} {} - {} ({})",
                    "•".blue(),
                    run.run_id.cyan(),
                    format_status(&run.status),
                    run.started_at
                );
            }
        }
    }

    Ok(())
}

async fn cancel_run(client: ApiClient, run_id: String, _output: &OutputFormat) -> Result<()> {
    let _spinner = spinner::new("Cancelling run...");

    client.post(&format!("/api/runs/{}/cancel", run_id), &()).await?;

    drop(_spinner);

    println!("{} Run cancelled: {}", "✓".green(), run_id.cyan());

    Ok(())
}

fn format_status(status: &str) -> colored::ColoredString {
    match status {
        "running" => status.yellow(),
        "completed" => status.green(),
        "failed" => status.red(),
        "cancelled" => status.bright_black(),
        _ => status.normal(),
    }
}
