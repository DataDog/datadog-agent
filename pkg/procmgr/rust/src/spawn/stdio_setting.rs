// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Result, bail};
use std::path::PathBuf;

/// Portable stdio setting from processes.d YAML (`inherit`, `null`, or a file path).
///
/// Platform spawn backends resolve this to `std::process::Stdio` or Win32 handles
/// under the target process identity.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum StdioSetting {
    Inherit,
    Null,
    File(PathBuf),
}

impl StdioSetting {
    pub fn is_inherit_or_null(&self) -> bool {
        matches!(self, Self::Inherit | Self::Null)
    }
}

/// Parse a processes.d stdout/stderr YAML value.
pub fn parse_stdio_setting(yaml_value: &str) -> StdioSetting {
    match yaml_value {
        "null" => StdioSetting::Null,
        "inherit" | "" => StdioSetting::Inherit,
        path => StdioSetting::File(PathBuf::from(path)),
    }
}

/// Reject privileged stdio settings that are not inherit/null.
pub(super) fn require_inherit_or_null(
    process_name: &str,
    stdout: &StdioSetting,
    stderr: &StdioSetting,
) -> Result<()> {
    if !stdout.is_inherit_or_null() || !stderr.is_inherit_or_null() {
        bail!("[{process_name}] refusing privileged spawn: stdout/stderr must be inherit or null");
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_stdio_setting_maps_keywords_and_paths() {
        assert_eq!(parse_stdio_setting("inherit"), StdioSetting::Inherit);
        assert_eq!(parse_stdio_setting(""), StdioSetting::Inherit);
        assert_eq!(parse_stdio_setting("null"), StdioSetting::Null);
        assert_eq!(
            parse_stdio_setting(r"C:\logs\out.log"),
            StdioSetting::File(PathBuf::from(r"C:\logs\out.log"))
        );
    }

    #[test]
    fn require_inherit_or_null_rejects_file_paths() {
        let file = StdioSetting::File(PathBuf::from(r"C:\logs\out.log"));
        let err = require_inherit_or_null("proc", &file, &StdioSetting::Inherit).unwrap_err();
        assert!(err.to_string().contains("inherit or null"));
    }

    #[test]
    fn is_inherit_or_null() {
        assert!(StdioSetting::Inherit.is_inherit_or_null());
        assert!(StdioSetting::Null.is_inherit_or_null());
        assert!(!StdioSetting::File(PathBuf::from("/var/log/out.log")).is_inherit_or_null());
    }
}
