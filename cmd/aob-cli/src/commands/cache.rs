use anyhow::Result;
use crate::client::ApiClient;
use crate::commands::CacheCommands;
use crate::OutputFormat;

pub async fn handle(
    _client: ApiClient,
    _command: CacheCommands,
    _output: &OutputFormat,
) -> Result<()> {
    // TODO: Implement cache commands
    println!("Cache commands not yet implemented");
    Ok(())
}
