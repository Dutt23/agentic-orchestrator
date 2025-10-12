use anyhow::Result;
use clap::{Parser, Subcommand};

mod auth;
mod client;
mod commands;
mod utils;

use auth::{ensure_authenticated, Config};
use client::ApiClient;
use commands::*;

#[derive(Parser)]
#[command(
    name = "aob",
    version,
    about = "Agentic Orchestration Builder CLI",
    long_about = "Command-line interface for managing workflows, runs, and approvals\n\n\
                  Examples:\n  \
                  aob run start workflow.json --inputs input.json\n  \
                  aob logs stream <run_id>\n  \
                  aob approve <ticket_id> approve\n\n\
                  For more help: aob help",
    after_help = "Use 'aob <command> --help' for more information about a command."
)]
#[command(args_conflicts_with_subcommands = false)]
#[command(arg_required_else_help = false)]
struct Cli {
    /// API endpoint URL
    #[arg(
        long,
        global = true,
        env = "AOB_API_URL",
        default_value = "http://localhost:8081"
    )]
    api_url: String,

    /// Output format
    #[arg(long, global = true, value_enum, default_value = "pretty")]
    output: OutputFormat,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Clone, clap::ValueEnum)]
enum OutputFormat {
    Pretty,
    Json,
    Compact,
}

#[derive(Subcommand)]
enum Commands {
    /// Start and manage workflow runs
    #[command(subcommand)]
    Run(RunCommands),

    /// Stream and view logs
    #[command(subcommand)]
    Logs(LogsCommands),

    /// Approve or reject HITL requests
    Approve {
        /// Approval ticket ID
        ticket_id: String,

        /// Decision (approve or reject)
        #[arg(value_enum)]
        decision: Decision,

        /// Reason for decision
        #[arg(long)]
        reason: Option<String>,
    },

    /// Manage patches
    #[command(subcommand)]
    Patch(PatchCommands),

    /// Manage workflows
    #[command(subcommand)]
    Workflow(WorkflowCommands),

    /// Manage artifacts
    #[command(subcommand)]
    Artifact(ArtifactCommands),

    /// Manage cache
    #[command(subcommand)]
    Cache(CacheCommands),

    /// Replay a run
    Replay {
        /// Run ID to replay
        run_id: String,

        /// Node to replay from
        #[arg(long)]
        from: Option<String>,

        /// Replay mode
        #[arg(long, value_enum, default_value = "freeze")]
        mode: ReplayMode,
    },

    /// Login with username
    Login {
        /// Username to use (optional, prompts if not provided)
        username: Option<String>,
    },

    /// Logout (clear stored credentials)
    Logout,

    /// Show current authentication status
    Whoami,
}

#[derive(Clone, clap::ValueEnum)]
enum Decision {
    Approve,
    Reject,
}

#[derive(Clone, clap::ValueEnum)]
enum ReplayMode {
    Freeze,
    Shadow,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Parse CLI arguments
    let cli = Cli::parse();

    // Handle auth commands that don't need API client
    match &cli.command {
        Commands::Login { username } => {
            return handle_login(username.clone()).await;
        }
        Commands::Logout => {
            return handle_logout();
        }
        Commands::Whoami => {
            return handle_whoami();
        }
        _ => {} // Continue to API commands
    }

    // Ensure user is authenticated
    let config = ensure_authenticated()?;

    // Initialize API client with username
    let client = ApiClient::new(&cli.api_url)?.with_username(config.username);

    // Execute command
    match cli.command {
        Commands::Run(cmd) => run::handle(client, cmd, &cli.output).await?,
        Commands::Logs(cmd) => logs::handle(client, cmd, &cli.output).await?,
        Commands::Approve {
            ticket_id,
            decision,
            reason,
        } => {
            approve::handle(
                client,
                ticket_id,
                matches!(decision, Decision::Approve),
                reason,
                &cli.output,
            )
            .await?
        }
        Commands::Patch(cmd) => patch::handle(client, cmd, &cli.output).await?,
        Commands::Workflow(cmd) => workflow::handle(client, cmd, &cli.output).await?,
        Commands::Artifact(cmd) => artifact::handle(client, cmd, &cli.output).await?,
        Commands::Cache(cmd) => cache::handle(client, cmd, &cli.output).await?,
        Commands::Replay { run_id, from, mode } => {
            replay::handle(
                client,
                run_id,
                from,
                matches!(mode, ReplayMode::Shadow),
                &cli.output,
            )
            .await?
        }
        Commands::Login { .. } | Commands::Logout | Commands::Whoami => {
            // Already handled above
            unreachable!()
        }
    }

    Ok(())
}

/// Handle login command
async fn handle_login(username: Option<String>) -> Result<()> {
    let username = match username {
        Some(u) => u,
        None => auth::prompt_username()?,
    };

    let config = Config { username };
    config.save()?;

    println!("✓ Logged in as: {}", config.username);
    println!("✓ Config saved to: {}", Config::config_path_display());

    Ok(())
}

/// Handle logout command
fn handle_logout() -> Result<()> {
    // Check if logged in
    let config = Config::load()?;

    if config.is_none() {
        println!("Not logged in");
        return Ok(());
    }

    Config::delete()?;
    println!("✓ Logged out successfully");
    println!("✓ Config deleted from: {}", Config::config_path_display());

    Ok(())
}

/// Handle whoami command
fn handle_whoami() -> Result<()> {
    match Config::load()? {
        Some(config) => {
            println!("Logged in as: {}", config.username);
            println!("Config file: {}", Config::config_path_display());
        }
        None => {
            println!("Not logged in");
            println!("Run 'aob login' to authenticate");
        }
    }

    Ok(())
}
