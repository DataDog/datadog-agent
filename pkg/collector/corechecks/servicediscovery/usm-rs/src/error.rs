use thiserror::Error;

/// USM-specific error types
#[derive(Error, Debug)]
pub enum UsmError {
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("JSON parsing error: {0}")]
    Json(#[from] serde_json::Error),

    #[error("ZIP archive error: {0}")]
    Zip(#[from] zip::result::ZipError),

    #[error("File system error: {0}")]
    FileSystem(String),

    #[error("Invalid path: {0}")]
    InvalidPath(String),

    #[error("Parse error: {0}")]
    Parse(String),

    #[error("Detection failed: {0}")]
    Detection(String),

    #[error("Configuration error: {0}")]
    Config(String),

    #[cfg(feature = "yaml")]
    #[error("YAML error: {0}")]
    Yaml(#[from] serde_yaml::Error),

    #[cfg(feature = "properties")]
    #[error("TOML error: {0}")]
    Toml(#[from] toml::de::Error),
}

pub type UsmResult<T> = Result<T, UsmError>;

/// Extension trait for Result to add graceful fallback methods
pub trait ResultExt<T> {
    /// Log error and return None instead of propagating error
    fn log_and_continue(self) -> Option<T>;
    
    /// Log error and return default value instead of propagating error
    fn log_and_default(self) -> T where T: Default;
    
    /// Log error and return provided fallback value
    fn log_and_fallback(self, fallback: T) -> T;
    
    /// Continue with provided fallback function on error
    fn or_else_log<F>(self, fallback_fn: F) -> T where F: FnOnce() -> T;
}

impl<T> ResultExt<T> for UsmResult<T> {
    fn log_and_continue(self) -> Option<T> {
        match self {
            Ok(value) => Some(value),
            Err(err) => {
                tracing::debug!("Operation failed, continuing: {}", err);
                None
            }
        }
    }
    
    fn log_and_default(self) -> T where T: Default {
        match self {
            Ok(value) => value,
            Err(err) => {
                tracing::debug!("Operation failed, using default: {}", err);
                T::default()
            }
        }
    }
    
    fn log_and_fallback(self, fallback: T) -> T {
        match self {
            Ok(value) => value,
            Err(err) => {
                tracing::debug!("Operation failed, using fallback: {}", err);
                fallback
            }
        }
    }
    
    fn or_else_log<F>(self, fallback_fn: F) -> T where F: FnOnce() -> T {
        match self {
            Ok(value) => value,
            Err(err) => {
                tracing::debug!("Operation failed, computing fallback: {}", err);
                fallback_fn()
            }
        }
    }
}