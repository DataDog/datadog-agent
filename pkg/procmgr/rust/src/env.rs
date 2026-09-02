// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::{Context, Result};
use log::warn;

pub(crate) fn expand_env_vars(input: &str) -> String {
    expand_vars_with(input, |name| std::env::var(name).ok())
}

pub(crate) fn try_expand_env_vars(input: &str) -> Option<String> {
    try_expand_vars_with(input, |name| std::env::var(name).ok())
}

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
                out.push_str(&rest[start..]);
                return out;
            }
        }
    }
    out.push_str(rest);
    out
}

pub(crate) fn try_expand_vars_with(
    input: &str,
    lookup: impl Fn(&str) -> Option<String>,
) -> Option<String> {
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(start) = rest.find("${") {
        out.push_str(&rest[..start]);
        let after = &rest[start + 2..];
        let end = after.find('}')?;
        let name = &after[..end];
        out.push_str(&lookup(name)?);
        rest = &after[end + 1..];
    }
    out.push_str(rest);
    Some(out)
}

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

    #[test]
    fn test_try_expand_vars_substitutes_known() {
        let lookup = |name: &str| match name {
            "DD_INVENTORIES_FIRST_RUN_DELAY" => Some("5".to_string()),
            _ => None,
        };
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            Some("5".to_string())
        );
    }

    #[test]
    fn test_try_expand_vars_none_when_unset() {
        let lookup = |_: &str| None;
        assert_eq!(
            try_expand_vars_with("${DD_INVENTORIES_FIRST_RUN_DELAY}", lookup),
            None,
            "an unset optional variable must yield None so the caller can omit the env var"
        );
    }
}
