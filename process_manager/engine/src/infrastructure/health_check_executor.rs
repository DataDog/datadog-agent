//! Health check executor implementation
//! Performs HTTP, TCP, and Exec health checks

use crate::domain::ports::HealthCheckExecutor;
use crate::domain::{DomainError, HealthCheck, HealthCheckType, HealthStatus};
use async_trait::async_trait;
use std::process::Command;
use std::time::Duration;
use tracing::{debug, error, warn};

/// Standard health check executor
pub struct StandardHealthCheckExecutor;

impl StandardHealthCheckExecutor {
    pub fn new() -> Self {
        Self
    }
}

impl Default for StandardHealthCheckExecutor {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl HealthCheckExecutor for StandardHealthCheckExecutor {
    async fn check(&self, config: &HealthCheck) -> Result<HealthStatus, DomainError> {
        let timeout = Duration::from_secs(config.timeout);

        match config.check_type {
            HealthCheckType::Http => perform_http_check(config, timeout).await,
            HealthCheckType::Tcp => perform_tcp_check(config, timeout).await,
            HealthCheckType::Exec => perform_exec_check(config, timeout).await,
        }
    }
}

/// HTTP health check
async fn perform_http_check(
    config: &HealthCheck,
    timeout: Duration,
) -> Result<HealthStatus, DomainError> {
    let endpoint = match &config.http_endpoint {
        Some(e) => e,
        None => {
            error!("HTTP health check missing endpoint");
            return Ok(HealthStatus::Unhealthy);
        }
    };

    let method = config.http_method.as_deref().unwrap_or("GET");
    let expected_status = config.http_expected_status.unwrap_or(200);

    debug!(
        "Performing HTTP health check: {} {} (expecting {})",
        method, endpoint, expected_status
    );

    // Use ureq for synchronous HTTP (simpler than async reqwest for now)
    // We'll spawn this in tokio::task::spawn_blocking to avoid blocking
    let endpoint_clone = endpoint.clone();
    let method_clone = method.to_string();

    let result = tokio::task::spawn_blocking(move || {
        let agent = ureq::AgentBuilder::new().timeout(timeout).build();

        let response = match method_clone.to_uppercase().as_str() {
            "GET" => agent.get(&endpoint_clone).call(),
            "POST" => agent.post(&endpoint_clone).call(),
            "HEAD" => agent.head(&endpoint_clone).call(),
            "PUT" => agent.put(&endpoint_clone).call(),
            "DELETE" => agent.delete(&endpoint_clone).call(),
            "PATCH" => agent.patch(&endpoint_clone).call(),
            _ => {
                warn!("Unsupported HTTP method: {}, using GET", method_clone);
                agent.get(&endpoint_clone).call()
            }
        };

        match response {
            Ok(resp) => {
                let status = resp.status();
                debug!("HTTP health check response: {}", status);
                if status == expected_status {
                    HealthStatus::Healthy
                } else {
                    warn!(
                        "HTTP health check failed: expected {}, got {}",
                        expected_status, status
                    );
                    HealthStatus::Unhealthy
                }
            }
            Err(ureq::Error::Status(code, _)) => {
                warn!("HTTP health check failed with status: {}", code);
                if code == expected_status {
                    HealthStatus::Healthy
                } else {
                    HealthStatus::Unhealthy
                }
            }
            Err(e) => {
                warn!("HTTP health check error: {}", e);
                HealthStatus::Unhealthy
            }
        }
    })
    .await;

    match result {
        Ok(status) => Ok(status),
        Err(e) => {
            error!("HTTP health check task failed: {}", e);
            Ok(HealthStatus::Unhealthy)
        }
    }
}

/// TCP health check
async fn perform_tcp_check(
    config: &HealthCheck,
    timeout: Duration,
) -> Result<HealthStatus, DomainError> {
    let host = match &config.tcp_host {
        Some(h) => h,
        None => {
            error!("TCP health check missing host");
            return Ok(HealthStatus::Unhealthy);
        }
    };

    let port = match config.tcp_port {
        Some(p) => p,
        None => {
            error!("TCP health check missing port");
            return Ok(HealthStatus::Unhealthy);
        }
    };

    let addr = format!("{}:{}", host, port);
    debug!("Performing TCP health check: {}", addr);

    match tokio::time::timeout(timeout, tokio::net::TcpStream::connect(&addr)).await {
        Ok(Ok(_stream)) => {
            debug!("TCP health check succeeded: {}", addr);
            Ok(HealthStatus::Healthy)
        }
        Ok(Err(e)) => {
            warn!("TCP health check connection failed: {} - {}", addr, e);
            Ok(HealthStatus::Unhealthy)
        }
        Err(_) => {
            warn!("TCP health check timed out: {}", addr);
            Ok(HealthStatus::Unhealthy)
        }
    }
}

/// Exec health check
async fn perform_exec_check(
    config: &HealthCheck,
    _timeout: Duration,
) -> Result<HealthStatus, DomainError> {
    let command = match &config.exec_command {
        Some(c) => c,
        None => {
            error!("Exec health check missing command");
            return Ok(HealthStatus::Unhealthy);
        }
    };

    let args = config.exec_args.clone().unwrap_or_default();
    debug!("Performing Exec health check: {} {:?}", command, args);

    let command_clone = command.clone();
    let args_clone = args.clone();

    let result = tokio::task::spawn_blocking(move || {
        let mut cmd = Command::new(&command_clone);
        cmd.args(&args_clone);

        // Execute with timeout simulation (Command doesn't have built-in timeout)
        match cmd.output() {
            Ok(output) => {
                if output.status.success() {
                    debug!(
                        "Exec health check succeeded: {} {:?}",
                        command_clone, args_clone
                    );
                    HealthStatus::Healthy
                } else {
                    let exit_code = output.status.code().unwrap_or(-1);
                    warn!(
                        "Exec health check failed: {} {:?} (exit code: {})",
                        command_clone, args_clone, exit_code
                    );
                    HealthStatus::Unhealthy
                }
            }
            Err(e) => {
                warn!(
                    "Exec health check error: {} {:?} - {}",
                    command_clone, args_clone, e
                );
                HealthStatus::Unhealthy
            }
        }
    })
    .await;

    match result {
        Ok(status) => Ok(status),
        Err(e) => {
            error!("Exec health check task failed: {}", e);
            Ok(HealthStatus::Unhealthy)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_tcp_check_success() {
        // Start a simple TCP server
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();

        tokio::spawn(async move {
            let (_socket, _) = listener.accept().await.unwrap();
        });

        let config = HealthCheck::tcp("127.0.0.1".to_string(), addr.port()).with_timeout(5);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Healthy);
    }

    #[tokio::test]
    async fn test_tcp_check_failure() {
        let config = HealthCheck::tcp("127.0.0.1".to_string(), 9).with_timeout(1);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Unhealthy);
    }

    #[tokio::test]
    #[cfg(unix)]
    async fn test_exec_check_success() {
        let config = HealthCheck::exec("true".to_string(), vec![]);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Healthy);
    }

    #[tokio::test]
    #[cfg(unix)]
    async fn test_exec_check_failure() {
        let config = HealthCheck::exec("false".to_string(), vec![]);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Unhealthy);
    }

    #[tokio::test]
    #[cfg(windows)]
    async fn test_exec_check_success() {
        // On Windows, use cmd /c exit 0 to simulate 'true'
        let config = HealthCheck::exec("cmd".to_string(), vec!["/c".to_string(), "exit".to_string(), "0".to_string()]);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Healthy);
    }

    #[tokio::test]
    #[cfg(windows)]
    async fn test_exec_check_failure() {
        // On Windows, use cmd /c exit 1 to simulate 'false'
        let config = HealthCheck::exec("cmd".to_string(), vec!["/c".to_string(), "exit".to_string(), "1".to_string()]);

        let executor = StandardHealthCheckExecutor::new();
        let status = executor.check(&config).await.unwrap();
        assert_eq!(status, HealthStatus::Unhealthy);
    }
}
