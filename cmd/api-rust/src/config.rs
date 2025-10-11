use std::{collections::HashMap, env};

#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
    pub services: HashMap<String, String>,
    pub rate_limit: u32,
    pub log_level: String,
}

impl Config {
    pub fn from_env() -> Self {
        let mut services = HashMap::new();

        // Register backend services
        services.insert(
            "orchestrator".to_string(),
            env::var("ORCHESTRATOR_URL").unwrap_or_else(|_| "http://localhost:8080".to_string()),
        );
        services.insert(
            "runner".to_string(),
            env::var("RUNNER_URL").unwrap_or_else(|_| "http://localhost:8082".to_string()),
        );
        services.insert(
            "hitl".to_string(),
            env::var("HITL_URL").unwrap_or_else(|_| "http://localhost:8083".to_string()),
        );
        services.insert(
            "parser".to_string(),
            env::var("PARSER_URL").unwrap_or_else(|_| "http://localhost:8084".to_string()),
        );
        services.insert(
            "fanout".to_string(),
            env::var("FANOUT_URL").unwrap_or_else(|_| "http://localhost:8085".to_string()),
        );

        Self {
            port: env::var("PORT")
                .unwrap_or_else(|_| "8081".to_string())
                .parse()
                .expect("PORT must be a valid number"),

            services,

            rate_limit: env::var("RATE_LIMIT")
                .unwrap_or_else(|_| "1000".to_string())
                .parse()
                .expect("RATE_LIMIT must be a valid number"),

            log_level: env::var("LOG_LEVEL")
                .unwrap_or_else(|_| "info".to_string()),
        }
    }

    pub fn get_service_url(&self, service: &str) -> Option<&String> {
        self.services.get(service)
    }
}
