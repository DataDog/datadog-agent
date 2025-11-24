//! KillMode Value Object
//!
//! Defines how child processes are terminated (systemd-compatible)

use serde::{Deserialize, Serialize};
use std::fmt;

/// KillMode determines how child processes are terminated
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, Default)]
pub enum KillMode {
    /// Kill all processes in cgroup (or process-group as fallback) - Default
    /// This is the most comprehensive cleanup method
    #[default]
    ControlGroup,

    /// Kill entire process group
    /// Uses the process group ID to kill all related processes
    ProcessGroup,

    /// Kill only the main process
    /// Child processes may become orphaned
    Process,

    /// Mixed mode: SIGTERM to main process, SIGKILL to rest of group
    /// Gives main process a chance to cleanup, but forcefully kills children
    Mixed,
}

impl KillMode {
    /// Parse a KillMode from a string
    pub fn parse(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "control-group" | "controlgroup" | "cgroup" => Some(Self::ControlGroup),
            "process-group" | "processgroup" => Some(Self::ProcessGroup),
            "process" | "main" => Some(Self::Process),
            "mixed" => Some(Self::Mixed),
            _ => None,
        }
    }

    /// Returns true if this mode requires process group support
    pub fn requires_process_group(&self) -> bool {
        matches!(self, Self::ProcessGroup | Self::Mixed | Self::ControlGroup)
    }

    /// Returns true if this mode requires cgroup support
    pub fn requires_cgroup(&self) -> bool {
        matches!(self, Self::ControlGroup)
    }
}

impl fmt::Display for KillMode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        let s = match self {
            Self::ControlGroup => "control-group",
            Self::ProcessGroup => "process-group",
            Self::Process => "process",
            Self::Mixed => "mixed",
        };
        write!(f, "{}", s)
    }
}

impl std::str::FromStr for KillMode {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Self::parse(s).ok_or_else(|| {
            format!(
                "Invalid kill mode: '{}'. Valid options: control-group, process-group, process, mixed",
                s
            )
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default() {
        assert_eq!(KillMode::default(), KillMode::ControlGroup);
    }

    #[test]
    fn test_parse() {
        assert_eq!(
            KillMode::parse("control-group"),
            Some(KillMode::ControlGroup)
        );
        assert_eq!(
            KillMode::parse("controlgroup"),
            Some(KillMode::ControlGroup)
        );
        assert_eq!(KillMode::parse("cgroup"), Some(KillMode::ControlGroup));
        assert_eq!(
            KillMode::parse("process-group"),
            Some(KillMode::ProcessGroup)
        );
        assert_eq!(
            KillMode::parse("processgroup"),
            Some(KillMode::ProcessGroup)
        );
        assert_eq!(KillMode::parse("process"), Some(KillMode::Process));
        assert_eq!(KillMode::parse("main"), Some(KillMode::Process));
        assert_eq!(KillMode::parse("mixed"), Some(KillMode::Mixed));
        assert_eq!(KillMode::parse("invalid"), None);
    }

    #[test]
    fn test_display() {
        assert_eq!(KillMode::ControlGroup.to_string(), "control-group");
        assert_eq!(KillMode::ProcessGroup.to_string(), "process-group");
        assert_eq!(KillMode::Process.to_string(), "process");
        assert_eq!(KillMode::Mixed.to_string(), "mixed");
    }

    #[test]
    fn test_from_str() {
        assert_eq!(
            "control-group".parse::<KillMode>().unwrap(),
            KillMode::ControlGroup
        );
        assert_eq!(
            "process-group".parse::<KillMode>().unwrap(),
            KillMode::ProcessGroup
        );
        assert!("invalid".parse::<KillMode>().is_err());
    }

    #[test]
    fn test_requires_process_group() {
        assert!(KillMode::ControlGroup.requires_process_group());
        assert!(KillMode::ProcessGroup.requires_process_group());
        assert!(KillMode::Mixed.requires_process_group());
        assert!(!KillMode::Process.requires_process_group());
    }

    #[test]
    fn test_requires_cgroup() {
        assert!(KillMode::ControlGroup.requires_cgroup());
        assert!(!KillMode::ProcessGroup.requires_cgroup());
        assert!(!KillMode::Process.requires_cgroup());
        assert!(!KillMode::Mixed.requires_cgroup());
    }
}
