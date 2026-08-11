// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::warn;

/// Expand `${VAR}` references in `input` using dd-procmgr's own environment.
///
/// This lets a single process definition be pointed at the stable or experiment configuration
/// directory by the supervising dd-procmgr, which exports the target directory in its own
/// environment (the stable and experiment procmgr units export different values). It mirrors how
/// the datadog-agent stable/experiment units each select their own config directory, so the
/// experiment collector reads the experiment config while the process definition stays identical.
/// Unknown variables are left as the literal `${VAR}` and logged, so a misconfiguration surfaces
/// as a startup failure rather than silently resolving to an empty path.
pub(crate) fn expand_env_vars(input: &str) -> String {
    expand_vars_with(input, |name| std::env::var(name).ok())
}

/// Core of [`expand_env_vars`] with the variable lookup injected, so it can be unit-tested without
/// mutating the process environment.
pub(crate) fn expand_vars_with(input: &str, lookup: impl Fn(&str) -> Option<String>) -> String {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(start) = rest.find("${") {
        out.push_str(&rest[..start]);
        let after = &rest[start + 2..];
        match after.find('}') {
            Some(end) => {
                let name = &after[..end];
                match lookup(name) {
                    Some(val) => out.push_str(&val),
                    None => {
                        warn!(
                            "process config references unset variable ${{{name}}}, leaving it literal"
                        );
                        out.push_str(&rest[start..start + 2 + end + 1]);
                    }
                }
                rest = &after[end + 1..];
            }
            None => {
                // No closing brace: emit the remainder verbatim.
                out.push_str(&rest[start..]);
                return out;
            }
        }
    }
    out.push_str(rest);
    out
}

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
