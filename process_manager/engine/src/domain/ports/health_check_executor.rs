//! Port for health check execution
//! Defines the interface for performing health checks on processes

use crate::domain::{DomainError, HealthCheck, HealthStatus};
use async_trait::async_trait;

/// Port for executing health checks
#[async_trait]
pub trait HealthCheckExecutor: Send + Sync {
    /// Perform a health check based on the configuration
    async fn check(&self, config: &HealthCheck) -> Result<HealthStatus, DomainError>;
}
