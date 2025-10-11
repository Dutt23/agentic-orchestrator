pub mod approve;
pub mod artifact;
pub mod cache;
pub mod logs;
pub mod patch;
pub mod replay;
pub mod run;
pub mod workflow;

use clap::Subcommand;

#[derive(Subcommand)]
pub enum RunCommands {
    /// Start a new workflow run
    ///
    /// Examples:
    ///   aob run start workflow.json
    ///   aob run start workflow.json --inputs input.json
    ///   aob run start workflow.json -f  (follow logs)
    Start {
        /// Path to workflow JSON file
        workflow: String,

        /// Path to inputs JSON file (optional)
        #[arg(long)]
        inputs: Option<String>,

        /// Follow logs after starting
        #[arg(long, short)]
        follow: bool,
    },

    /// Get status of a run
    ///
    /// Shows run status, active nodes, start/end times
    Status {
        /// Run ID
        run_id: String,
    },

    /// List runs with optional filters
    ///
    /// Examples:
    ///   aob run list
    ///   aob run list --status running
    ///   aob run list --status completed --limit 50
    List {
        /// Filter by status
        #[arg(long)]
        status: Option<String>,

        /// Limit number of results
        #[arg(long, default_value = "10")]
        limit: usize,
    },

    /// Cancel a running workflow
    Cancel {
        /// Run ID to cancel
        run_id: String,
    },
}

#[derive(Subcommand)]
pub enum LogsCommands {
    /// Stream logs from a run (real-time SSE)
    ///
    /// Examples:
    ///   aob logs stream run_7f3e4a
    ///   aob logs stream run_7f3e4a --node parse
    ///   aob logs stream run_7f3e4a --filter errors
    Stream {
        /// Run ID
        run_id: String,

        /// Filter by node ID
        #[arg(long)]
        node: Option<String>,

        /// Filter level (all, errors)
        #[arg(long, default_value = "all")]
        filter: String,
    },
}

#[derive(Subcommand)]
pub enum PatchCommands {
    /// List patches for a run
    List {
        /// Run ID
        run_id: String,
    },

    /// Show patch details
    Show {
        /// Patch ID
        patch_id: String,
    },

    /// Approve a patch
    Approve {
        /// Patch ID
        patch_id: String,

        /// Reason for approval
        #[arg(long)]
        reason: Option<String>,
    },

    /// Reject a patch
    Reject {
        /// Patch ID
        patch_id: String,

        /// Reason for rejection
        #[arg(long)]
        reason: String,
    },
}

#[derive(Subcommand)]
pub enum WorkflowCommands {
    /// List workflows
    List,

    /// Validate a workflow file
    Validate {
        /// Path to workflow JSON file
        file: String,
    },

    /// Show workflow details
    Show {
        /// Workflow ID
        workflow_id: String,
    },
}

#[derive(Subcommand)]
pub enum ArtifactCommands {
    /// Get an artifact
    Get {
        /// Artifact reference (cas://sha256:...)
        artifact_ref: String,

        /// Output file path
        #[arg(long, short)]
        output: Option<String>,
    },

    /// List artifacts for a run
    List {
        /// Run ID
        run_id: String,
    },
}

#[derive(Subcommand)]
pub enum CacheCommands {
    /// Invalidate cache entry
    Invalidate {
        /// Cache key
        key: String,
    },

    /// Show cache statistics
    Stats,
}
