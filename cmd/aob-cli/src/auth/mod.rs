use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::fs;
use std::io::{self, Write};
use std::path::PathBuf;

/// User configuration stored in ~/.aob-cli/config.json
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub username: String,
}

impl Config {
    /// Get the config file path (~/.aob-cli/config.json)
    fn config_path() -> Result<PathBuf> {
        let home = dirs::home_dir().context("Could not find home directory")?;
        Ok(home.join(".aob-cli").join("config.json"))
    }

    /// Get the config directory path (~/.aob-cli/)
    fn config_dir() -> Result<PathBuf> {
        let home = dirs::home_dir().context("Could not find home directory")?;
        Ok(home.join(".aob-cli"))
    }

    /// Load config from file
    pub fn load() -> Result<Option<Self>> {
        let path = Self::config_path()?;

        if !path.exists() {
            return Ok(None);
        }

        let content = fs::read_to_string(&path)
            .with_context(|| format!("Failed to read config from {}", path.display()))?;

        let config: Config = serde_json::from_str(&content)
            .with_context(|| format!("Failed to parse config from {}", path.display()))?;

        Ok(Some(config))
    }

    /// Save config to file
    pub fn save(&self) -> Result<()> {
        let dir = Self::config_dir()?;
        let path = Self::config_path()?;

        // Create config directory if it doesn't exist
        if !dir.exists() {
            fs::create_dir_all(&dir)
                .with_context(|| format!("Failed to create config directory: {}", dir.display()))?;
        }

        // Write config file
        let content = serde_json::to_string_pretty(self)
            .context("Failed to serialize config")?;

        fs::write(&path, content)
            .with_context(|| format!("Failed to write config to {}", path.display()))?;

        Ok(())
    }

    /// Delete the config file (logout)
    pub fn delete() -> Result<()> {
        let path = Self::config_path()?;

        if path.exists() {
            fs::remove_file(&path)
                .with_context(|| format!("Failed to delete config at {}", path.display()))?;
        }

        Ok(())
    }

    /// Get config file path as string for display
    pub fn config_path_display() -> String {
        Self::config_path()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|_| "~/.aob-cli/config.json".to_string())
    }
}

/// Prompt user for username
pub fn prompt_username() -> Result<String> {
    print!("Enter your username: ");
    io::stdout().flush()?;

    let mut username = String::new();
    io::stdin()
        .read_line(&mut username)
        .context("Failed to read username")?;

    let username = username.trim().to_string();

    if username.is_empty() {
        anyhow::bail!("Username cannot be empty");
    }

    Ok(username)
}

/// Ensure user is authenticated, prompt if not
pub fn ensure_authenticated() -> Result<Config> {
    // Try to load existing config
    if let Some(config) = Config::load()? {
        return Ok(config);
    }

    // No config found, prompt for login
    eprintln!("⚠️  Not logged in. Please enter your credentials.");
    eprintln!();

    let username = prompt_username()?;

    let config = Config { username };
    config.save()?;

    eprintln!();
    eprintln!("✓ Logged in as: {}", config.username);
    eprintln!("✓ Config saved to: {}", Config::config_path_display());
    eprintln!();

    Ok(config)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_config_serialization() {
        let config = Config {
            username: "test-user".to_string(),
        };

        let json = serde_json::to_string(&config).unwrap();
        let parsed: Config = serde_json::from_str(&json).unwrap();

        assert_eq!(config.username, parsed.username);
    }
}
