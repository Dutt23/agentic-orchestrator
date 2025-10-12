use anyhow::{Context, Result};
use reqwest::Client;
use serde::{de::DeserializeOwned, Serialize};
use std::time::Duration;

pub struct ApiClient {
    base_url: String,
    client: Client,
    username: Option<String>,
}

impl ApiClient {
    pub fn new(base_url: &str) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(30))
            .connect_timeout(Duration::from_secs(10))
            .build()
            .context("Failed to create HTTP client")?;

        Ok(Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            client,
            username: None,
        })
    }

    /// Set the username for X-User-ID header
    pub fn with_username(mut self, username: String) -> Self {
        self.username = Some(username);
        self
    }

    /// Get the username
    pub fn username(&self) -> Option<&str> {
        self.username.as_deref()
    }

    pub async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T> {
        let url = format!("{}{}", self.base_url, path);

        let mut request = self.client.get(&url);

        // Add X-User-ID header if username is set
        if let Some(username) = &self.username {
            request = request.header("X-User-ID", username);
        }

        let response = request
            .send()
            .await
            .context("Request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("API error ({}): {}", status, body);
        }

        response.json().await.context("Failed to parse response")
    }

    pub async fn post<T: Serialize, R: DeserializeOwned>(
        &self,
        path: &str,
        body: &T,
    ) -> Result<R> {
        let url = format!("{}{}", self.base_url, path);

        let mut request = self.client.post(&url).json(body);

        // Add X-User-ID header if username is set
        if let Some(username) = &self.username {
            request = request.header("X-User-ID", username);
        }

        let response = request
            .send()
            .await
            .context("Request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("API error ({}): {}", status, body);
        }

        response.json().await.context("Failed to parse response")
    }

    #[allow(dead_code)]
    pub async fn delete(&self, path: &str) -> Result<()> {
        let url = format!("{}{}", self.base_url, path);

        let mut request = self.client.delete(&url);

        // Add X-User-ID header if username is set
        if let Some(username) = &self.username {
            request = request.header("X-User-ID", username);
        }

        let response = request
            .send()
            .await
            .context("Request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("API error ({}): {}", status, body);
        }

        Ok(())
    }

    pub fn sse_url(&self, path: &str) -> String {
        format!("{}{}", self.base_url, path)
    }

    #[allow(dead_code)]
    pub fn base_url(&self) -> &str {
        &self.base_url
    }
}
