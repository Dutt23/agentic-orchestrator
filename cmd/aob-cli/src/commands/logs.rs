use anyhow::{Context, Result};
use colored::Colorize;
use eventsource_stream::Eventsource;
use futures::StreamExt;
use serde::Deserialize;

use crate::client::ApiClient;
use crate::commands::LogsCommands;
use crate::OutputFormat;

#[derive(Deserialize)]
struct LogEvent {
    timestamp: String,
    level: String,
    node_id: Option<String>,
    message: String,
}

pub async fn handle(
    client: ApiClient,
    command: LogsCommands,
    _output: &OutputFormat,
) -> Result<()> {
    match command {
        LogsCommands::Stream {
            run_id,
            node,
            filter,
        } => stream_logs(client, run_id, node, filter).await,
    }
}

pub async fn stream_logs(
    client: ApiClient,
    run_id: String,
    node: Option<String>,
    filter: String,
) -> Result<()> {
    let mut url = format!("/api/logs/stream?run_id={}&filter={}", run_id, filter);
    if let Some(node) = node {
        url.push_str(&format!("&node_id={}", node));
    }

    let sse_url = client.sse_url(&url);

    println!(
        "{} Streaming logs for run: {}",
        "•".blue(),
        run_id.cyan()
    );
    println!("Press Ctrl+C to stop\n");

    let http_client = reqwest::Client::new();
    let response = http_client
        .get(&sse_url)
        .send()
        .await
        .context("Failed to connect to log stream")?;

    let mut stream = response.bytes_stream().eventsource();

    while let Some(event) = stream.next().await {
        match event {
            Ok(event) => {
                if event.event == "log" {
                    if let Ok(log) = serde_json::from_str::<LogEvent>(&event.data) {
                        print_log(&log);
                    }
                } else if event.event == "error" {
                    eprintln!("{} Stream error: {}", "✗".red(), event.data);
                    break;
                } else if event.event == "done" {
                    println!("\n{} Stream ended", "✓".green());
                    break;
                }
            }
            Err(e) => {
                eprintln!("{} Stream error: {}", "✗".red(), e);
                break;
            }
        }
    }

    Ok(())
}

fn print_log(log: &LogEvent) {
    let level_str = match log.level.as_str() {
        "error" => "ERROR".red(),
        "warn" => "WARN".yellow(),
        "info" => "INFO".blue(),
        "debug" => "DEBUG".bright_black(),
        _ => log.level.as_str().normal(),
    };

    let node_str = if let Some(node) = &log.node_id {
        format!("[{}] ", node.cyan())
    } else {
        String::new()
    };

    println!(
        "{} {} {}{}",
        log.timestamp.bright_black(),
        level_str,
        node_str,
        log.message
    );
}
