//! ProcessType value object
//! Defines the type/behavior of a process (systemd-style)

use serde::{Deserialize, Serialize};
use std::fmt;

/// Type of process (systemd-style service types)
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, Default)]
pub enum ProcessType {
    /// Simple process that runs in the foreground
    /// The process is considered started immediately
    #[default]
    Simple,

    /// Process that forks a child and exits
    /// We track the child PID
    Forking,

    /// Process that runs once and exits
    /// Used for initialization tasks
    Oneshot,

    /// Process that sends a notification when ready
    /// Uses sd_notify protocol
    Notify,
}

impl ProcessType {
    /// Check if this process type is expected to exit naturally
    pub fn expects_exit(&self) -> bool {
        matches!(self, ProcessType::Oneshot)
    }

    /// Check if we need to wait for a ready notification
    pub fn needs_ready_notification(&self) -> bool {
        matches!(self, ProcessType::Notify)
    }

    /// Check if we need to track forked child PIDs
    pub fn tracks_child_pid(&self) -> bool {
        matches!(self, ProcessType::Forking)
    }

    /// Parse from string representation (systemd-style)
    pub fn parse(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "simple" => Some(ProcessType::Simple),
            "forking" => Some(ProcessType::Forking),
            "oneshot" | "one-shot" => Some(ProcessType::Oneshot),
            "notify" => Some(ProcessType::Notify),
            _ => None,
        }
    }
}

impl fmt::Display for ProcessType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ProcessType::Simple => write!(f, "simple"),
            ProcessType::Forking => write!(f, "forking"),
            ProcessType::Oneshot => write!(f, "oneshot"),
            ProcessType::Notify => write!(f, "notify"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_expects_exit() {
        assert!(ProcessType::Oneshot.expects_exit());
        assert!(!ProcessType::Simple.expects_exit());
        assert!(!ProcessType::Forking.expects_exit());
        assert!(!ProcessType::Notify.expects_exit());
    }

    #[test]
    fn test_needs_ready_notification() {
        assert!(ProcessType::Notify.needs_ready_notification());
        assert!(!ProcessType::Simple.needs_ready_notification());
        assert!(!ProcessType::Forking.needs_ready_notification());
        assert!(!ProcessType::Oneshot.needs_ready_notification());
    }

    #[test]
    fn test_tracks_child_pid() {
        assert!(ProcessType::Forking.tracks_child_pid());
        assert!(!ProcessType::Simple.tracks_child_pid());
        assert!(!ProcessType::Oneshot.tracks_child_pid());
        assert!(!ProcessType::Notify.tracks_child_pid());
    }

    #[test]
    fn test_from_str() {
        assert_eq!(ProcessType::parse("simple"), Some(ProcessType::Simple));
        assert_eq!(ProcessType::parse("forking"), Some(ProcessType::Forking));
        assert_eq!(ProcessType::parse("oneshot"), Some(ProcessType::Oneshot));
        assert_eq!(ProcessType::parse("one-shot"), Some(ProcessType::Oneshot));
        assert_eq!(ProcessType::parse("notify"), Some(ProcessType::Notify));
        assert_eq!(ProcessType::parse("invalid"), None);
    }

    #[test]
    fn test_from_str_case_insensitive() {
        assert_eq!(ProcessType::parse("SIMPLE"), Some(ProcessType::Simple));
        assert_eq!(ProcessType::parse("Forking"), Some(ProcessType::Forking));
        assert_eq!(ProcessType::parse("ONESHOT"), Some(ProcessType::Oneshot));
    }

    #[test]
    fn test_display() {
        assert_eq!(ProcessType::Simple.to_string(), "simple");
        assert_eq!(ProcessType::Forking.to_string(), "forking");
        assert_eq!(ProcessType::Oneshot.to_string(), "oneshot");
        assert_eq!(ProcessType::Notify.to_string(), "notify");
    }

    #[test]
    fn test_default() {
        assert_eq!(ProcessType::default(), ProcessType::Simple);
    }
}
