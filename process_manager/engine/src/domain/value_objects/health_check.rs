//! Health check value objects
//! Domain model for process health checking

use serde::{Deserialize, Serialize};

/// Type of health check to perform
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum HealthCheckType {
    #[default]
    Http,
    Tcp,
    Exec,
}

impl std::fmt::Display for HealthCheckType {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Http => write!(f, "http"),
            Self::Tcp => write!(f, "tcp"),
            Self::Exec => write!(f, "exec"),
        }
    }
}

/// Health check configuration value object
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HealthCheck {
    pub check_type: HealthCheckType,

    // Timing configuration
    pub interval: u64,     // seconds between checks
    pub timeout: u64,      // seconds before timeout
    pub retries: u32,      // consecutive failures before unhealthy
    pub start_period: u64, // grace period after start (seconds)

    // Action configuration
    pub restart_after: u32, // kill and restart after N failures (0 = never)

    // HTTP-specific fields
    pub http_endpoint: Option<String>,
    pub http_method: Option<String>,
    pub http_expected_status: Option<u16>,

    // TCP-specific fields
    pub tcp_host: Option<String>,
    pub tcp_port: Option<u16>,

    // Exec-specific fields
    pub exec_command: Option<String>,
    pub exec_args: Option<Vec<String>>,
}

impl HealthCheck {
    /// Create a new HTTP health check
    pub fn http(endpoint: String) -> Self {
        Self {
            check_type: HealthCheckType::Http,
            interval: 30,
            timeout: 5,
            retries: 3,
            start_period: 0,
            restart_after: 0,
            http_endpoint: Some(endpoint),
            http_method: Some("GET".to_string()),
            http_expected_status: Some(200),
            tcp_host: None,
            tcp_port: None,
            exec_command: None,
            exec_args: None,
        }
    }

    /// Create a new TCP health check
    pub fn tcp(host: String, port: u16) -> Self {
        Self {
            check_type: HealthCheckType::Tcp,
            interval: 30,
            timeout: 5,
            retries: 3,
            start_period: 0,
            restart_after: 0,
            http_endpoint: None,
            http_method: None,
            http_expected_status: None,
            tcp_host: Some(host),
            tcp_port: Some(port),
            exec_command: None,
            exec_args: None,
        }
    }

    /// Create a new Exec health check
    pub fn exec(command: String, args: Vec<String>) -> Self {
        Self {
            check_type: HealthCheckType::Exec,
            interval: 30,
            timeout: 5,
            retries: 3,
            start_period: 0,
            restart_after: 0,
            http_endpoint: None,
            http_method: None,
            http_expected_status: None,
            tcp_host: None,
            tcp_port: None,
            exec_command: Some(command),
            exec_args: Some(args),
        }
    }

    /// Builder method to set interval
    pub fn with_interval(mut self, interval: u64) -> Self {
        self.interval = interval;
        self
    }

    /// Builder method to set timeout
    pub fn with_timeout(mut self, timeout: u64) -> Self {
        self.timeout = timeout;
        self
    }

    /// Builder method to set retries
    pub fn with_retries(mut self, retries: u32) -> Self {
        self.retries = retries;
        self
    }

    /// Builder method to set start_period
    pub fn with_start_period(mut self, start_period: u64) -> Self {
        self.start_period = start_period;
        self
    }

    /// Builder method to set restart_after
    pub fn with_restart_after(mut self, restart_after: u32) -> Self {
        self.restart_after = restart_after;
        self
    }

    /// Validate health check configuration
    pub fn validate(&self) -> Result<(), String> {
        match self.check_type {
            HealthCheckType::Http => {
                if self.http_endpoint.is_none() {
                    return Err("HTTP health check requires endpoint".to_string());
                }
            }
            HealthCheckType::Tcp => {
                if self.tcp_host.is_none() {
                    return Err("TCP health check requires host".to_string());
                }
                if self.tcp_port.is_none() {
                    return Err("TCP health check requires port".to_string());
                }
            }
            HealthCheckType::Exec => {
                if self.exec_command.is_none() {
                    return Err("Exec health check requires command".to_string());
                }
            }
        }
        Ok(())
    }
}

/// Health status of a process
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
#[serde(rename_all = "lowercase")]
pub enum HealthStatus {
    Healthy,
    Unhealthy,
    Starting, // Within start_period, not yet checking
    #[default]
    Unknown, // Not yet checked or no health check configured
}

impl std::fmt::Display for HealthStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Healthy => write!(f, "healthy"),
            Self::Unhealthy => write!(f, "unhealthy"),
            Self::Starting => write!(f, "starting"),
            Self::Unknown => write!(f, "unknown"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_http_health_check_creation() {
        let hc = HealthCheck::http("http://localhost:8080/health".to_string())
            .with_interval(10)
            .with_retries(5);

        assert_eq!(hc.check_type, HealthCheckType::Http);
        assert_eq!(hc.interval, 10);
        assert_eq!(hc.retries, 5);
        assert!(hc.http_endpoint.is_some());
        assert!(hc.validate().is_ok());
    }

    #[test]
    fn test_tcp_health_check_creation() {
        let hc = HealthCheck::tcp("127.0.0.1".to_string(), 5432).with_timeout(3);

        assert_eq!(hc.check_type, HealthCheckType::Tcp);
        assert_eq!(hc.timeout, 3);
        assert!(hc.tcp_host.is_some());
        assert!(hc.tcp_port.is_some());
        assert!(hc.validate().is_ok());
    }

    #[test]
    fn test_exec_health_check_creation() {
        let hc = HealthCheck::exec("/bin/health-check".to_string(), vec!["--fast".to_string()]);

        assert_eq!(hc.check_type, HealthCheckType::Exec);
        assert!(hc.exec_command.is_some());
        assert!(hc.validate().is_ok());
    }

    #[test]
    fn test_invalid_http_health_check() {
        let mut hc = HealthCheck::http("http://localhost/health".to_string());
        hc.http_endpoint = None;

        assert!(hc.validate().is_err());
    }

    #[test]
    fn test_health_status_display() {
        assert_eq!(HealthStatus::Healthy.to_string(), "healthy");
        assert_eq!(HealthStatus::Unhealthy.to_string(), "unhealthy");
        assert_eq!(HealthStatus::Starting.to_string(), "starting");
        assert_eq!(HealthStatus::Unknown.to_string(), "unknown");
    }
}
