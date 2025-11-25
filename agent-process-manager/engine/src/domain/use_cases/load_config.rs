//! LoadConfig Use Case
//!
//! Loads process definitions from YAML configuration files

use crate::domain::services::{
    ConfigParsingService, ProcessCreationService, ProcessLifecycleService,
};
use crate::domain::{DomainError, LoadConfigCommand, LoadConfigResponse};
use crate::infrastructure::load_config_from_path;
use async_trait::async_trait;
use std::sync::Arc;
use tracing::{info, warn};

/// LoadConfig use case trait
#[async_trait]
pub trait LoadConfig: Send + Sync {
    async fn execute(&self, command: LoadConfigCommand) -> Result<LoadConfigResponse, DomainError>;
}

/// LoadConfig use case implementation
pub struct LoadConfigUseCase {
    creation_service: Arc<ProcessCreationService>,
    lifecycle_service: Arc<ProcessLifecycleService>,
}

impl LoadConfigUseCase {
    pub fn new(
        creation_service: Arc<ProcessCreationService>,
        lifecycle_service: Arc<ProcessLifecycleService>,
    ) -> Self {
        Self {
            creation_service,
            lifecycle_service,
        }
    }
}

#[async_trait]
impl LoadConfig for LoadConfigUseCase {
    async fn execute(&self, command: LoadConfigCommand) -> Result<LoadConfigResponse, DomainError> {
        info!(config_path = %command.config_path, "Loading configuration");

        // Load YAML config(s)
        let configs = load_config_from_path(&command.config_path)
            .map_err(|e| DomainError::InvalidCommand(format!("Failed to load config: {}", e)))?;

        if configs.is_empty() {
            warn!("No processes found in configuration");
            return Ok(LoadConfigResponse {
                processes_created: 0,
                processes_started: 0,
                errors: Vec::new(),
            });
        }

        info!(
            process_count = configs.len(),
            "Found processes in configuration"
        );

        let mut created_count = 0;
        let mut started_count = 0;
        let mut errors = Vec::new();

        // Create each process
        for (name, config) in configs {
            info!(name = %name, command = %config.command, "Creating process from config");

            let should_auto_start = config.auto_start; // Capture before moving config

            // Parse YAML config to CreateProcessCommand using ConfigParser
            let create_cmd = match ConfigParsingService::parse(name.clone(), config) {
                Ok(cmd) => cmd,
                Err(e) => {
                    let msg = format!("Failed to parse config for '{}': {}", name, e);
                    warn!("{}", msg);
                    errors.push(msg);
                    continue;
                }
            };

            // Create the process
            match self.creation_service.create_from_command(create_cmd).await {
                Ok((id, _name)) => {
                    created_count += 1;
                    info!(name = %name, "Process created successfully");

                    // Auto-start if configured
                    if should_auto_start {
                        info!(name = %name, "Auto-starting process from config");

                        // Delegate to lifecycle service (handles hooks + spawn + register)
                        match self
                            .lifecycle_service
                            .spawn_and_register(&id, vec![], 0)
                            .await
                        {
                            Ok(()) => {
                                started_count += 1;
                                info!(name = %name, "Process auto-started successfully");
                            }
                            Err(e) => {
                                let msg = format!("Failed to auto-start '{}': {}", name, e);
                                warn!("{}", msg);
                                errors.push(msg);
                            }
                        }
                    }
                }
                Err(e) => {
                    let msg = format!("Failed to create process '{}': {}", name, e);
                    warn!("{}", msg);
                    errors.push(msg);
                }
            }
        }

        info!(
            created = created_count,
            started = started_count,
            errors = errors.len(),
            "Configuration loading completed"
        );

        Ok(LoadConfigResponse {
            processes_created: created_count,
            processes_started: started_count,
            errors,
        })
    }
}
