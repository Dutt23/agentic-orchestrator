use anyhow::Result;
use clap::{Parser, Subcommand};

mod client;
mod commands;
mod utils;

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
struct Cli {
    /// API endpoint URL
    #[arg(
        long,
        env = "AOB_API_URL",
        default_value = "http://localhost:8081"
    )]
    api_url: String,

    /// Output format
    #[arg(long, value_enum, default_value = "pretty")]
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
    /// Show detailed help and examples
    Help,

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

    // Initialize API client
    let client = ApiClient::new(&cli.api_url)?;

    // Execute command
    match cli.command {
        Commands::Help => {
            print_help();
            return Ok(());
        }
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
    }

    Ok(())
}

fn print_help() {
    use colored::Colorize;

    println!("{}", "Agentic Orchestration Builder (aob) - CLI Help".bold().cyan());
    println!();
    println!("{}", "QUICK START".bold());
    println!("  1. Start a workflow:   {}", "aob run start workflow.json".yellow());
    println!("  2. Check status:       {}", "aob run status <run_id>".yellow());
    println!("  3. Stream logs:        {}", "aob logs stream <run_id>".yellow());
    println!();

    println!("{}", "COMMON COMMANDS".bold());
    println!();

    println!("  {} - Workflow Execution", "run".green().bold());
    println!("    {:<30} Start a new workflow run", "aob run start <file.json>".yellow());
    println!("    {:<30} Start with inputs", "aob run start <file> --inputs <inputs.json>".yellow());
    println!("    {:<30} Start and follow logs", "aob run start <file> -f".yellow());
    println!("    {:<30} Get run status", "aob run status <run_id>".yellow());
    println!("    {:<30} List recent runs", "aob run list".yellow());
    println!("    {:<30} Cancel a run", "aob run cancel <run_id>".yellow());
    println!();

    println!("  {} - Log Streaming", "logs".green().bold());
    println!("    {:<30} Stream all logs", "aob logs stream <run_id>".yellow());
    println!("    {:<30} Filter by node", "aob logs stream <run_id> --node <node_id>".yellow());
    println!("    {:<30} Show only errors", "aob logs stream <run_id> --filter errors".yellow());
    println!();

    println!("  {} - Human Approvals", "approve".green().bold());
    println!("    {:<30} Approve a request", "aob approve <ticket_id> approve".yellow());
    println!("    {:<30} Reject with reason", "aob approve <ticket_id> reject --reason \"...\"".yellow());
    println!();

    println!("  {} - Patch Management", "patch".green().bold());
    println!("    {:<30} List patches for run", "aob patch list <run_id>".yellow());
    println!("    {:<30} Show patch details", "aob patch show <patch_id>".yellow());
    println!("    {:<30} Approve a patch", "aob patch approve <patch_id>".yellow());
    println!("    {:<30} Reject a patch", "aob patch reject <patch_id> --reason \"...\"".yellow());
    println!();

    println!("  {} - Replay & Debug", "replay".green().bold());
    println!("    {:<30} Replay entire run", "aob replay <run_id>".yellow());
    println!("    {:<30} Replay from node", "aob replay <run_id> --from <node_id>".yellow());
    println!("    {:<30} Shadow mode (dry-run)", "aob replay <run_id> --mode shadow".yellow());
    println!();

    println!("  {} - Workflows", "workflow".green().bold());
    println!("    {:<30} List workflows", "aob workflow list".yellow());
    println!("    {:<30} Validate workflow file", "aob workflow validate <file.json>".yellow());
    println!("    {:<30} Show workflow details", "aob workflow show <workflow_id>".yellow());
    println!();

    println!("  {} - Artifacts & Cache", "artifact/cache".green().bold());
    println!("    {:<30} Get artifact", "aob artifact get <cas://...>".yellow());
    println!("    {:<30} List run artifacts", "aob artifact list <run_id>".yellow());
    println!("    {:<30} Invalidate cache", "aob cache invalidate <key>".yellow());
    println!("    {:<30} Show cache stats", "aob cache stats".yellow());
    println!();

    println!("{}", "CONFIGURATION".bold());
    println!("  Set API endpoint: {}", "export AOB_API_URL=http://localhost:8081".yellow());
    println!("  Or use flag:      {}", "aob --api-url <url> <command>".yellow());
    println!();

    println!("{}", "OUTPUT FORMATS".bold());
    println!("  Default (pretty):  {}", "aob run status <run_id>".yellow());
    println!("  JSON output:       {}", "aob run status <run_id> --output json".yellow());
    println!("  Compact output:    {}", "aob run status <run_id> --output compact".yellow());
    println!();

    println!("{}", "EXAMPLES".bold());
    println!();
    println!("  {} Complete workflow lifecycle", "1.".bold());
    println!("     aob run start examples/lead-flow.json --inputs lead.json -f");
    println!("     # Agent proposes patch -> shows patch_abc123");
    println!("     aob patch show patch_abc123");
    println!("     aob patch approve patch_abc123");
    println!("     # HITL approval -> shows ticket_456");
    println!("     aob approve ticket_456 approve --reason \"Data verified\"");
    println!();

    println!("  {} Debug a failed run", "2.".bold());
    println!("     aob run status run_7f3e4a");
    println!("     aob logs stream run_7f3e4a --filter errors");
    println!("     aob replay run_7f3e4a --from failing_node");
    println!();

    println!("  {} Monitor multiple runs", "3.".bold());
    println!("     aob run list --status running");
    println!("     # Open multiple terminals with:");
    println!("     aob logs stream run_1 & aob logs stream run_2 &");
    println!();

    println!("{}", "MORE HELP".bold());
    println!("  Command help:   {}", "aob <command> --help".yellow());
    println!("  Subcommand:     {}", "aob run --help".yellow());
    println!("  Documentation:  https://github.com/lyzr/orchestrator");
    println!();
}
