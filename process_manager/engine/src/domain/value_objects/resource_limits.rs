//! ResourceLimits value object
//! Defines resource limits for processes (CPU, memory, PIDs)

use serde::{Deserialize, Serialize};
use std::fmt;

/// Resource limits for a process
/// Inspired by Kubernetes resource limits and systemd resource control
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct ResourceLimits {
    /// CPU limit in millicores (e.g., 1000 = 1 core, 500 = 0.5 cores)
    /// Used to set CPU quotas via cgroups
    pub cpu_millis: Option<u64>,

    /// Memory limit in bytes
    /// Used to set memory limits via cgroups
    pub memory_bytes: Option<u64>,

    /// Maximum number of PIDs (processes/threads)
    /// Used to set pids.max via cgroups
    pub max_pids: Option<u32>,
}

impl ResourceLimits {
    /// Create new resource limits
    pub fn new() -> Self {
        Self::default()
    }

    /// Set CPU limit in millicores
    pub fn with_cpu_millis(mut self, millis: u64) -> Self {
        self.cpu_millis = Some(millis);
        self
    }

    /// Set memory limit in bytes
    pub fn with_memory_bytes(mut self, bytes: u64) -> Self {
        self.memory_bytes = Some(bytes);
        self
    }

    /// Set max PIDs
    pub fn with_max_pids(mut self, pids: u32) -> Self {
        self.max_pids = Some(pids);
        self
    }

    /// Check if any limits are set
    pub fn has_limits(&self) -> bool {
        self.cpu_millis.is_some() || self.memory_bytes.is_some() || self.max_pids.is_some()
    }

    /// Parse CPU string to millicores
    /// Examples: "500m" -> 500, "1" -> 1000, "2.5" -> 2500
    pub fn parse_cpu(cpu_str: &str) -> Result<u64, String> {
        if let Some(m) = cpu_str.strip_suffix('m') {
            // Millicores: "500m" -> 500
            m.parse::<u64>()
                .map_err(|e| format!("Invalid CPU millicores: {}", e))
        } else {
            // Cores: "1" -> 1000, "2.5" -> 2500
            let cores: f64 = cpu_str
                .parse()
                .map_err(|e| format!("Invalid CPU cores: {}", e))?;
            Ok((cores * 1000.0) as u64)
        }
    }

    /// Parse memory string to bytes
    /// Examples: "256M" -> 268435456, "1G" -> 1073741824, "512K" -> 524288
    pub fn parse_memory(mem_str: &str) -> Result<u64, String> {
        let mem_str = mem_str.trim();
        if mem_str.is_empty() {
            return Err("Empty memory string".to_string());
        }

        // Check for unit suffix
        let (value_str, multiplier) = if let Some(v) = mem_str.strip_suffix('K') {
            (v, 1024_u64)
        } else if let Some(v) = mem_str.strip_suffix('M') {
            (v, crate::domain::constants::BYTES_PER_MB)
        } else if let Some(v) = mem_str.strip_suffix('G') {
            (v, crate::domain::constants::BYTES_PER_GB)
        } else if let Some(v) = mem_str.strip_suffix('T') {
            (v, crate::domain::constants::BYTES_PER_TB)
        } else {
            // No suffix, assume bytes
            (mem_str, 1_u64)
        };

        let value: u64 = value_str
            .parse()
            .map_err(|e| format!("Invalid memory value: {}", e))?;

        Ok(value * multiplier)
    }
}

impl fmt::Display for ResourceLimits {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let mut parts = Vec::new();

        if let Some(cpu) = self.cpu_millis {
            if cpu % 1000 == 0 {
                parts.push(format!("cpu={}cores", cpu / 1000));
            } else {
                parts.push(format!("cpu={}m", cpu));
            }
        }

        if let Some(mem) = self.memory_bytes {
            // Display in human-readable format
            if mem >= crate::domain::constants::BYTES_PER_GB {
                parts.push(format!(
                    "memory={}G",
                    mem / crate::domain::constants::BYTES_PER_GB
                ));
            } else if mem >= crate::domain::constants::BYTES_PER_MB {
                parts.push(format!(
                    "memory={}M",
                    mem / crate::domain::constants::BYTES_PER_MB
                ));
            } else if mem >= crate::domain::constants::BYTES_PER_KB {
                parts.push(format!(
                    "memory={}K",
                    mem / crate::domain::constants::BYTES_PER_KB
                ));
            } else {
                parts.push(format!("memory={}B", mem));
            }
        }

        if let Some(pids) = self.max_pids {
            parts.push(format!("pids={}", pids));
        }

        if parts.is_empty() {
            write!(f, "no-limits")
        } else {
            write!(f, "{}", parts.join(", "))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_cpu_millicores() {
        assert_eq!(ResourceLimits::parse_cpu("500m").unwrap(), 500);
        assert_eq!(ResourceLimits::parse_cpu("1000m").unwrap(), 1000);
        assert_eq!(ResourceLimits::parse_cpu("2500m").unwrap(), 2500);
    }

    #[test]
    fn test_parse_cpu_cores() {
        assert_eq!(ResourceLimits::parse_cpu("1").unwrap(), 1000);
        assert_eq!(ResourceLimits::parse_cpu("2").unwrap(), 2000);
        assert_eq!(ResourceLimits::parse_cpu("2.5").unwrap(), 2500);
        assert_eq!(ResourceLimits::parse_cpu("0.5").unwrap(), 500);
    }

    #[test]
    fn test_parse_cpu_invalid() {
        assert!(ResourceLimits::parse_cpu("abc").is_err());
        assert!(ResourceLimits::parse_cpu("").is_err());
    }

    #[test]
    fn test_parse_memory_bytes() {
        assert_eq!(ResourceLimits::parse_memory("1024").unwrap(), 1024);
        assert_eq!(ResourceLimits::parse_memory("512").unwrap(), 512);
    }

    #[test]
    fn test_parse_memory_kilobytes() {
        assert_eq!(ResourceLimits::parse_memory("1K").unwrap(), 1024);
        assert_eq!(ResourceLimits::parse_memory("512K").unwrap(), 512 * 1024);
    }

    #[test]
    fn test_parse_memory_megabytes() {
        assert_eq!(ResourceLimits::parse_memory("1M").unwrap(), 1024 * 1024);
        assert_eq!(
            ResourceLimits::parse_memory("256M").unwrap(),
            256 * 1024 * 1024
        );
    }

    #[test]
    fn test_parse_memory_gigabytes() {
        assert_eq!(
            ResourceLimits::parse_memory("1G").unwrap(),
            1024 * 1024 * 1024
        );
        assert_eq!(
            ResourceLimits::parse_memory("2G").unwrap(),
            2 * 1024 * 1024 * 1024
        );
    }

    #[test]
    fn test_parse_memory_terabytes() {
        assert_eq!(
            ResourceLimits::parse_memory("1T").unwrap(),
            1024_u64 * 1024 * 1024 * 1024
        );
    }

    #[test]
    fn test_parse_memory_invalid() {
        assert!(ResourceLimits::parse_memory("abc").is_err());
        assert!(ResourceLimits::parse_memory("").is_err());
    }

    #[test]
    fn test_has_limits() {
        let limits = ResourceLimits::new();
        assert!(!limits.has_limits());

        let limits = ResourceLimits::new().with_cpu_millis(1000);
        assert!(limits.has_limits());

        let limits = ResourceLimits::new().with_memory_bytes(1024 * 1024 * 256);
        assert!(limits.has_limits());

        let limits = ResourceLimits::new().with_max_pids(100);
        assert!(limits.has_limits());
    }

    #[test]
    fn test_display() {
        let limits = ResourceLimits::new();
        assert_eq!(limits.to_string(), "no-limits");

        let limits = ResourceLimits::new().with_cpu_millis(1000);
        assert_eq!(limits.to_string(), "cpu=1cores");

        let limits = ResourceLimits::new().with_cpu_millis(500);
        assert_eq!(limits.to_string(), "cpu=500m");

        let limits = ResourceLimits::new().with_memory_bytes(256 * 1024 * 1024);
        assert_eq!(limits.to_string(), "memory=256M");

        let limits = ResourceLimits::new().with_max_pids(100);
        assert_eq!(limits.to_string(), "pids=100");

        let limits = ResourceLimits::new()
            .with_cpu_millis(2000)
            .with_memory_bytes(512 * 1024 * 1024)
            .with_max_pids(200);
        assert_eq!(limits.to_string(), "cpu=2cores, memory=512M, pids=200");
    }

    #[test]
    fn test_builder() {
        let limits = ResourceLimits::new()
            .with_cpu_millis(1500)
            .with_memory_bytes(1024 * 1024 * 512)
            .with_max_pids(150);

        assert_eq!(limits.cpu_millis, Some(1500));
        assert_eq!(limits.memory_bytes, Some(1024 * 1024 * 512));
        assert_eq!(limits.max_pids, Some(150));
    }
}
