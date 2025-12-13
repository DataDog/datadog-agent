//! Infrastructure Layer
//!
//! This module contains the adapters that implement the ports defined in the domain layer.
//! These are the "driven adapters" (infrastructure implementations).
//!
//! ## Adapters
//!
//! - `TokioProcessExecutor`: Real system process execution using tokio
//! - `InMemoryProcessRepository`: Thread-safe in-memory storage for processes
//!
//! ## Usage
//!
//! ```rust,no_run
//! use pm_engine::infrastructure::{TokioProcessExecutor, InMemoryProcessRepository};
//! use std::sync::Arc;
//!
//! // Create real infrastructure
//! let executor = Arc::new(TokioProcessExecutor::new());
//! let repository = Arc::new(InMemoryProcessRepository::new());
//!
//! // Wire into use cases...
//! ```

pub mod config;
pub mod health_check_executor;
pub mod in_memory_repository;
pub mod tokio_executor;

pub use config::{
    get_default_config_path, load_config_from_path, Config, HealthCheckConfig, ProcessConfig,
    ResourceLimitsConfig,
};
pub use health_check_executor::StandardHealthCheckExecutor;
pub use in_memory_repository::InMemoryProcessRepository;
pub use tokio_executor::TokioProcessExecutor;
