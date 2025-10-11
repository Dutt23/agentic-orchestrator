use anyhow::Result;
use crate::client::ApiClient;
use crate::commands::ArtifactCommands;
use crate::OutputFormat;

pub async fn handle(
    _client: ApiClient,
    _command: ArtifactCommands,
    _output: &OutputFormat,
) -> Result<()> {
    // TODO: Implement artifact commands
    println!("Artifact commands not yet implemented");
    Ok(())
}
