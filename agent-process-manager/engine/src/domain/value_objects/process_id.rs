//! ProcessId value object
//! Immutable identifier for processes

use serde::{Deserialize, Serialize};
use std::fmt;
use uuid::Uuid;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub struct ProcessId(Uuid);

impl ProcessId {
    /// Generate a new random ProcessId
    pub fn generate() -> Self {
        Self(Uuid::new_v4())
    }

    /// Create from existing UUID
    pub fn from_uuid(uuid: Uuid) -> Self {
        Self(uuid)
    }

    /// Parse from string
    pub fn from_string(s: &str) -> Result<Self, uuid::Error> {
        Ok(Self(Uuid::parse_str(s)?))
    }

    /// Get inner UUID
    pub fn as_uuid(&self) -> &Uuid {
        &self.0
    }
}

impl fmt::Display for ProcessId {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl From<Uuid> for ProcessId {
    fn from(uuid: Uuid) -> Self {
        Self(uuid)
    }
}

impl From<ProcessId> for Uuid {
    fn from(id: ProcessId) -> Self {
        id.0
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate() {
        let id1 = ProcessId::generate();
        let id2 = ProcessId::generate();
        assert_ne!(id1, id2);
    }

    #[test]
    fn test_from_string() {
        let uuid_str = "550e8400-e29b-41d4-a716-446655440000";
        let id = ProcessId::from_string(uuid_str).unwrap();
        assert_eq!(id.to_string(), uuid_str);
    }

    #[test]
    fn test_display() {
        let id = ProcessId::generate();
        let displayed = format!("{}", id);
        assert!(!displayed.is_empty());
    }
}
