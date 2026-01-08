//! LoadConfig Use Case
//!
//! Loads process definitions from YAML configuration files

use crate::domain::services::ConfigParsingService;
use crate::domain::services::ProcessCreationService;
use crate::domain::use_cases::StartProcess;
use crate::domain::{DomainError, LoadConfigCommand, LoadConfigResponse, StartProcessCommand};
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
    start_process: Arc<dyn StartProcess>,
}

impl LoadConfigUseCase {
    pub fn new(
        creation_service: Arc<ProcessCreationService>,
        start_process: Arc<dyn StartProcess>,
    ) -> Self {
        Self {
            creation_service,
            start_process,
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

        // Two-pass approach:
        // 1. First pass: Create ALL processes (so wants: dependencies exist)
        // 2. Second pass: Auto-start processes (StartProcess handles wants: dependencies)

        // Track which processes should be auto-started
        let mut to_auto_start: Vec<String> = Vec::new();

        // First pass: Create all processes
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
                Ok((_id, created_name)) => {
                    created_count += 1;
                    info!(name = %name, "Process created successfully");

                    // Track for auto-start in second pass
                    if should_auto_start {
                        to_auto_start.push(created_name);
                    }
                }
                Err(e) => {
                    let msg = format!("Failed to create process '{}': {}", name, e);
                    warn!("{}", msg);
                    errors.push(msg);
                }
            }
        }

        // Second pass: Auto-start processes using StartProcess use case
        // This properly handles wants: dependencies (auto-starts them too)
        for name in to_auto_start {
            info!(name = %name, "Auto-starting process from config");

            let start_cmd = StartProcessCommand::from_name(name.clone());
            match self.start_process.execute(start_cmd).await {
                Ok(_) => {
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
