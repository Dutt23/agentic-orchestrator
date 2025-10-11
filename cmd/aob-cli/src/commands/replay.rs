use anyhow::Result;
use crate::client::ApiClient;
use crate::OutputFormat;

pub async fn handle(
    _client: ApiClient,
    _run_id: String,
    _from: Option<String>,
    _shadow: bool,
    _output: &OutputFormat,
) -> Result<()> {
    // TODO: Implement replay command
    println!("Replay command not yet implemented");
    Ok(())
}
