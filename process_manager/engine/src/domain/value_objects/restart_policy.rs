//! RestartPolicy value object
//! Defines when a process should be automatically restarted

use serde::{Deserialize, Serialize};
use std::fmt;

/// Policy for automatically restarting processes
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, Default)]
pub enum RestartPolicy {
    /// Never restart the process
    #[default]
    Never,

    /// Always restart, regardless of exit code
    Always,

    /// Restart only on failure (non-zero exit code)
    OnFailure,

    /// Restart only on success (zero exit code)
    OnSuccess,
}

impl RestartPolicy {
    /// Check if the process should be restarted given an exit code
    pub fn should_restart(&self, exit_code: i32) -> bool {
        match self {
            RestartPolicy::Never => false,
            RestartPolicy::Always => true,
            RestartPolicy::OnFailure => exit_code != 0,
            RestartPolicy::OnSuccess => exit_code == 0,
        }
    }

    /// Parse from string representation (systemd-style)
    pub fn parse(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "never" | "no" => Some(RestartPolicy::Never),
            "always" => Some(RestartPolicy::Always),
            "on-failure" | "onfailure" => Some(RestartPolicy::OnFailure),
            "on-success" | "onsuccess" => Some(RestartPolicy::OnSuccess),
            _ => None,
        }
    }
}

impl fmt::Display for RestartPolicy {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            RestartPolicy::Never => write!(f, "never"),
            RestartPolicy::Always => write!(f, "always"),
            RestartPolicy::OnFailure => write!(f, "on-failure"),
            RestartPolicy::OnSuccess => write!(f, "on-success"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_never_restart() {
        let policy = RestartPolicy::Never;
        assert!(!policy.should_restart(0));
        assert!(!policy.should_restart(1));
        assert!(!policy.should_restart(127));
    }

    #[test]
    fn test_always_restart() {
        let policy = RestartPolicy::Always;
        assert!(policy.should_restart(0));
        assert!(policy.should_restart(1));
        assert!(policy.should_restart(127));
    }

    #[test]
    fn test_on_failure_restart() {
        let policy = RestartPolicy::OnFailure;
        assert!(!policy.should_restart(0)); // Success - no restart
        assert!(policy.should_restart(1)); // Failure - restart
        assert!(policy.should_restart(127)); // Failure - restart
    }

    #[test]
    fn test_on_success_restart() {
        let policy = RestartPolicy::OnSuccess;
        assert!(policy.should_restart(0)); // Success - restart
        assert!(!policy.should_restart(1)); // Failure - no restart
        assert!(!policy.should_restart(127)); // Failure - no restart
    }

    #[test]
    fn test_from_str() {
        assert_eq!(RestartPolicy::parse("never"), Some(RestartPolicy::Never));
        assert_eq!(RestartPolicy::parse("no"), Some(RestartPolicy::Never));
        assert_eq!(RestartPolicy::parse("always"), Some(RestartPolicy::Always));
        assert_eq!(
            RestartPolicy::parse("on-failure"),
            Some(RestartPolicy::OnFailure)
        );
        assert_eq!(
            RestartPolicy::parse("onfailure"),
            Some(RestartPolicy::OnFailure)
        );
        assert_eq!(
            RestartPolicy::parse("on-success"),
            Some(RestartPolicy::OnSuccess)
        );
        assert_eq!(
            RestartPolicy::parse("onsuccess"),
            Some(RestartPolicy::OnSuccess)
        );
        assert_eq!(RestartPolicy::parse("invalid"), None);
    }

    #[test]
    fn test_from_str_case_insensitive() {
        assert_eq!(RestartPolicy::parse("NEVER"), Some(RestartPolicy::Never));
        assert_eq!(RestartPolicy::parse("Always"), Some(RestartPolicy::Always));
        assert_eq!(
            RestartPolicy::parse("ON-FAILURE"),
            Some(RestartPolicy::OnFailure)
        );
    }

    #[test]
    fn test_display() {
        assert_eq!(RestartPolicy::Never.to_string(), "never");
        assert_eq!(RestartPolicy::Always.to_string(), "always");
        assert_eq!(RestartPolicy::OnFailure.to_string(), "on-failure");
        assert_eq!(RestartPolicy::OnSuccess.to_string(), "on-success");
    }

    #[test]
    fn test_default() {
        assert_eq!(RestartPolicy::default(), RestartPolicy::Never);
    }
}
