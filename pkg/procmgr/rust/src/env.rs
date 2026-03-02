// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};

/// Parse a systemd-style environment file into key-value pairs.
/// Supports `KEY=VALUE`, `KEY="VALUE"`, `KEY='VALUE'`, comments (#), and blank lines.
pub fn parse_environment_file(path: &str) -> Result<Vec<(String, String)>> {
    let contents = std::fs::read_to_string(path)
        .with_context(|| format!("reading environment file: {path}"))?;
    let mut vars = Vec::new();
    for line in contents.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        if let Some((key, raw_val)) = trimmed.split_once('=') {
            let val = raw_val
                .trim()
                .trim_matches('"')
                .trim_matches('\'')
                .to_string();
            vars.push((key.trim().to_string(), val));
        }
    }
    Ok(vars)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_parse_full() {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("env");
        std::fs::write(
            &path,
            r#"# Datadog env
DD_API_KEY=abc123
PATH="/usr/local/bin:/usr/bin"
QUOTED='single'
malformed line without equals
   
# blank lines above are skipped
LANG=en_US.UTF-8
"#,
        )
        .unwrap();

        let vars: HashMap<String, String> = parse_environment_file(path.to_str().unwrap())
            .unwrap()
            .into_iter()
            .collect();

        assert_eq!(vars["DD_API_KEY"], "abc123");
        assert_eq!(vars["PATH"], "/usr/local/bin:/usr/bin");
        assert_eq!(vars["QUOTED"], "single");
        assert_eq!(vars["LANG"], "en_US.UTF-8");
        assert_eq!(vars.len(), 4, "malformed line should be silently skipped");
    }

    #[test]
    fn test_parse_missing_file() {
        assert!(parse_environment_file("/nonexistent/env").is_err());
    }
}
