//! LoadConfig Command

/// Command to load configuration
pub struct LoadConfigCommand {
    pub config_path: String,
}

/// Response from loading configuration
#[derive(Debug)]
pub struct LoadConfigResponse {
    pub processes_created: usize,
    pub processes_started: usize,
    pub errors: Vec<String>,
}
