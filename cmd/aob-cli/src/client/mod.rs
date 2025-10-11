use anyhow::{Context, Result};
use reqwest::Client;
use serde::{de::DeserializeOwned, Serialize};
use std::time::Duration;

pub struct ApiClient {
    base_url: String,
    client: Client,
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
        })
    }

    pub async fn get<T: DeserializeOwned>(&self, path: &str) -> Result<T> {
        let url = format!("{}{}", self.base_url, path);

        let response = self
            .client
            .get(&url)
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

        let response = self
            .client
            .post(&url)
            .json(body)
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

    pub async fn delete(&self, path: &str) -> Result<()> {
        let url = format!("{}{}", self.base_url, path);

        let response = self
            .client
            .delete(&url)
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

    pub fn base_url(&self) -> &str {
        &self.base_url
    }
}
