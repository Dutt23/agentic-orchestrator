use anyhow::Result;
use colored::Colorize;
use serde::{Deserialize, Serialize};

use crate::client::ApiClient;
use crate::commands::WorkflowCommands;
use crate::OutputFormat;

#[derive(Deserialize, Serialize)]
struct WorkflowTag {
    tag_name: String,
    target_kind: String,
    target_id: String,
    target_hash: Option<String>,
    version: i64,
    moved_by: Option<String>,
    moved_at: String,
}

#[derive(Deserialize, Serialize)]
struct WorkflowListResponse {
    workflows: Vec<WorkflowTag>,
    count: usize,
}

pub async fn handle(
    client: ApiClient,
    command: WorkflowCommands,
    output: &OutputFormat,
) -> Result<()> {
    match command {
        WorkflowCommands::List => list_workflows(client, output).await,
        WorkflowCommands::Validate { file } => validate_workflow(client, file, output).await,
        WorkflowCommands::Show { workflow_id } => show_workflow(client, workflow_id, output).await,
    }
}

async fn list_workflows(client: ApiClient, output: &OutputFormat) -> Result<()> {
    let response: WorkflowListResponse = client.get("/api/v1/workflows").await?;

    match output {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(&response)?);
        }
        OutputFormat::Compact => {
            for tag in response.workflows {
                println!("{}\t{}\t{}\tv{}",
                    tag.tag_name,
                    tag.target_kind,
                    tag.target_id,
                    tag.version
                );
            }
        }
        OutputFormat::Pretty => {
            if response.workflows.is_empty() {
                println!("{}", "No workflows found".yellow());
                return Ok(());
            }

            println!("{} ({} total)", "Workflows:".bold().cyan(), response.count);
            println!();

            for tag in response.workflows {
                println!("{} {}",
                    "•".blue(),
                    tag.tag_name.cyan().bold()
                );

                println!("  Kind: {}", tag.target_kind.yellow());
                println!("  Target: {}", tag.target_id.bright_black());

                if let Some(hash) = &tag.target_hash {
                    println!("  Hash: {}", hash.bright_black());
                }

                println!("  Version: {}", tag.version.to_string().green());

                if let Some(moved_by) = &tag.moved_by {
                    println!("  Last updated by: {}", moved_by.bright_black());
                }

                println!("  Updated: {}", tag.moved_at.bright_black());

                println!();
            }
        }
    }

    Ok(())
}

async fn validate_workflow(
    _client: ApiClient,
    _file: String,
    _output: &OutputFormat,
) -> Result<()> {
    println!("Workflow validate command not yet implemented");
    Ok(())
}

async fn show_workflow(
    _client: ApiClient,
    _workflow_id: String,
    _output: &OutputFormat,
) -> Result<()> {
    println!("Workflow show command not yet implemented");
    Ok(())
}
