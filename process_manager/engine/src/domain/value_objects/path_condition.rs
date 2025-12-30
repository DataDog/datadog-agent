//! PathCondition value object
//! Represents conditions for process starting based on filesystem state

use serde::{Deserialize, Serialize};
use std::fmt;
use std::path::Path;

/// Conditional starting based on filesystem paths
/// Inspired by systemd's ConditionPathExists
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum PathCondition {
    /// Path must exist (AND logic - all Exists conditions must be true)
    Exists(String),

    /// Path must NOT exist (negation)
    NotExists(String),

    /// Path must exist (OR logic - at least one ExistsOr must be true)
    ExistsOr(String),
}

impl PathCondition {
    /// Parse a path condition from a string with optional prefix
    ///
    /// # Format
    /// - No prefix: `Exists` (AND logic) - e.g., "/var/run/app.sock"
    /// - "!" prefix: `NotExists` - e.g., "!/tmp/app.lock"
    /// - "|" prefix: `ExistsOr` (OR logic) - e.g., "|/etc/config.yaml"
    ///
    /// # Examples
    /// ```
    /// use pm_engine::domain::PathCondition;
    ///
    /// let exists = PathCondition::parse("/var/run/app.sock");
    /// let not_exists = PathCondition::parse("!/tmp/app.lock");
    /// let or_exists = PathCondition::parse("|/etc/config.yaml");
    /// ```
    pub fn parse(s: &str) -> Self {
        if let Some(path) = s.strip_prefix('!') {
            Self::NotExists(path.to_string())
        } else if let Some(path) = s.strip_prefix('|') {
            Self::ExistsOr(path.to_string())
        } else {
            Self::Exists(s.to_string())
        }
    }

    /// Get the path for this condition
    pub fn path(&self) -> &str {
        match self {
            Self::Exists(p) | Self::NotExists(p) | Self::ExistsOr(p) => p,
        }
    }

    /// Check if this condition is satisfied
    pub fn check(&self) -> bool {
        let exists = Path::new(self.path()).exists();
        match self {
            Self::Exists(_) => exists,
            Self::NotExists(_) => !exists,
            Self::ExistsOr(_) => exists,
        }
    }
}

impl fmt::Display for PathCondition {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Exists(p) => write!(f, "{}", p),
            Self::NotExists(p) => write!(f, "!{}", p),
            Self::ExistsOr(p) => write!(f, "|{}", p),
        }
    }
}

/// Check if all path conditions are satisfied
///
/// # Logic
/// - All `Exists` conditions must be true (AND)
/// - All `NotExists` conditions must be true (AND)
/// - At least one `ExistsOr` condition must be true (OR), if any are present
///
/// # Returns
/// - `true` if all conditions are satisfied
/// - `false` if any condition fails
pub fn check_all_conditions(conditions: &[PathCondition]) -> bool {
    if conditions.is_empty() {
        return true; // No conditions means always start
    }

    let mut has_or = false;
    let mut or_satisfied = false;

    for condition in conditions {
        match condition {
            PathCondition::Exists(_) | PathCondition::NotExists(_) => {
                // AND logic - must all be true
                if !condition.check() {
                    return false;
                }
            }
            PathCondition::ExistsOr(_) => {
                has_or = true;
                if condition.check() {
                    or_satisfied = true;
                }
            }
        }
    }

    // If there are OR conditions, at least one must be satisfied
    !has_or || or_satisfied
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_exists() {
        let condition = PathCondition::parse("/var/run/app.sock");
        assert_eq!(condition.path(), "/var/run/app.sock");
        assert!(matches!(condition, PathCondition::Exists(_)));
    }

    #[test]
    fn test_parse_not_exists() {
        let condition = PathCondition::parse("!/tmp/app.lock");
        assert_eq!(condition.path(), "/tmp/app.lock");
        assert!(matches!(condition, PathCondition::NotExists(_)));
    }

    #[test]
    fn test_parse_or_exists() {
        let condition = PathCondition::parse("|/etc/config.yaml");
        assert_eq!(condition.path(), "/etc/config.yaml");
        assert!(matches!(condition, PathCondition::ExistsOr(_)));
    }

    #[test]
    fn test_check_exists() {
        // Test with /tmp which should exist
        let condition = PathCondition::Exists("/tmp".to_string());
        assert!(condition.check());

        // Test with a path that doesn't exist
        let condition = PathCondition::Exists("/this/should/not/exist/ever".to_string());
        assert!(!condition.check());
    }

    #[test]
    fn test_check_not_exists() {
        // Test with a path that doesn't exist
        let condition = PathCondition::NotExists("/this/should/not/exist/ever".to_string());
        assert!(condition.check());

        // Test with /tmp which should exist
        let condition = PathCondition::NotExists("/tmp".to_string());
        assert!(!condition.check());
    }

    #[test]
    fn test_display() {
        assert_eq!(
            PathCondition::Exists("/path".to_string()).to_string(),
            "/path"
        );
        assert_eq!(
            PathCondition::NotExists("/path".to_string()).to_string(),
            "!/path"
        );
        assert_eq!(
            PathCondition::ExistsOr("/path".to_string()).to_string(),
            "|/path"
        );
    }

    #[test]
    fn test_check_all_conditions_empty() {
        assert!(check_all_conditions(&[]));
    }

    #[test]
    fn test_check_all_conditions_and_logic() {
        let conditions = vec![
            PathCondition::Exists("/tmp".to_string()),
            PathCondition::NotExists("/this/should/not/exist/ever".to_string()),
        ];
        assert!(check_all_conditions(&conditions));

        let conditions = vec![
            PathCondition::Exists("/tmp".to_string()),
            PathCondition::Exists("/this/should/not/exist/ever".to_string()),
        ];
        assert!(!check_all_conditions(&conditions));
    }

    #[test]
    fn test_check_all_conditions_or_logic() {
        // At least one OR must be true
        let conditions = vec![
            PathCondition::ExistsOr("/tmp".to_string()),
            PathCondition::ExistsOr("/this/should/not/exist/ever".to_string()),
        ];
        assert!(check_all_conditions(&conditions));

        // All OR false
        let conditions = vec![
            PathCondition::ExistsOr("/this/should/not/exist/1".to_string()),
            PathCondition::ExistsOr("/this/should/not/exist/2".to_string()),
        ];
        assert!(!check_all_conditions(&conditions));
    }

    #[test]
    fn test_check_all_conditions_mixed() {
        // AND + OR: all AND must be true, and at least one OR must be true
        let conditions = vec![
            PathCondition::Exists("/tmp".to_string()),
            PathCondition::NotExists("/this/should/not/exist/ever".to_string()),
            PathCondition::ExistsOr("/tmp".to_string()),
        ];
        assert!(check_all_conditions(&conditions));

        // AND fails
        let conditions = vec![
            PathCondition::Exists("/this/should/not/exist/ever".to_string()),
            PathCondition::ExistsOr("/tmp".to_string()),
        ];
        assert!(!check_all_conditions(&conditions));

        // OR fails
        let conditions = vec![
            PathCondition::Exists("/tmp".to_string()),
            PathCondition::ExistsOr("/this/should/not/exist/ever".to_string()),
        ];
        assert!(!check_all_conditions(&conditions));
    }
}
