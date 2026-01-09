// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs;
use std::sync::OnceLock;

use crate::procfs;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EphemeralPortType {
    Unknown = 0,
    Ephemeral = 1,
    NotEphemeral = 2,
}

#[derive(Debug, Clone, Copy)]
struct EphemeralRange {
    low: u16,
    high: u16,
}

impl EphemeralRange {
    fn new(low: u16, high: u16) -> Self {
        Self { low, high }
    }
}

static EPHEMERAL_RANGE: OnceLock<Option<EphemeralRange>> = OnceLock::new();

#[allow(clippy::get_first)]
fn init_ephemeral_range() -> Option<EphemeralRange> {
    let path = procfs::root_path().join("sys/net/ipv4/ip_local_port_range");

    if let Ok(content) = fs::read_to_string(path) {
        let parts: Vec<&str> = content.split_whitespace().collect();
        if parts.len() >= 2
            && let (Ok(low), Ok(high)) =
                (parts.get(0)?.parse::<u16>(), parts.get(1)?.parse::<u16>())
            && low > 0
            && high > 0
            && low <= high
        {
            return Some(EphemeralRange::new(low, high));
        }
    }

    None
}

/// Returns whether the port is ephemeral based on the OS-specific
/// configuration.
pub fn is_port_ephemeral(port: u16) -> EphemeralPortType {
    let range = EPHEMERAL_RANGE.get_or_init(init_ephemeral_range);
    classify_port(range, port)
}

fn classify_port(range: &Option<EphemeralRange>, port: u16) -> EphemeralPortType {
    match range {
        Some(r) if port >= r.low && port <= r.high => EphemeralPortType::Ephemeral,
        Some(_) => EphemeralPortType::NotEphemeral,
        None => EphemeralPortType::Unknown,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_classify_port_within_range() {
        let range = Some(EphemeralRange::new(32768, 65535));

        // Test ports within the ephemeral range
        assert_eq!(classify_port(&range, 32768), EphemeralPortType::Ephemeral);
        assert_eq!(classify_port(&range, 50000), EphemeralPortType::Ephemeral);
        assert_eq!(classify_port(&range, 65535), EphemeralPortType::Ephemeral);
    }

    #[test]
    fn test_classify_port_outside_range() {
        let range = Some(EphemeralRange::new(32768, 65535));

        // Test ports outside the ephemeral range
        assert_eq!(classify_port(&range, 80), EphemeralPortType::NotEphemeral);
        assert_eq!(classify_port(&range, 443), EphemeralPortType::NotEphemeral);
        assert_eq!(
            classify_port(&range, 32767),
            EphemeralPortType::NotEphemeral
        );
    }

    #[test]
    fn test_classify_port_no_range() {
        let range = None;

        // When range is not available, all ports should be Unknown
        assert_eq!(classify_port(&range, 80), EphemeralPortType::Unknown);
        assert_eq!(classify_port(&range, 50000), EphemeralPortType::Unknown);
        assert_eq!(classify_port(&range, 65535), EphemeralPortType::Unknown);
    }

    #[test]
    fn test_classify_port_edge_cases() {
        let range = Some(EphemeralRange::new(1024, 2048));

        // Test boundary values
        assert_eq!(classify_port(&range, 1023), EphemeralPortType::NotEphemeral);
        assert_eq!(classify_port(&range, 1024), EphemeralPortType::Ephemeral);
        assert_eq!(classify_port(&range, 2048), EphemeralPortType::Ephemeral);
        assert_eq!(classify_port(&range, 2049), EphemeralPortType::NotEphemeral);
    }
}
