// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! PHP service name detection.
//!
//! Detects service names from PHP processes by:
//! 1. Looking for `datadog.service` in CLI flags (-ddatadog.service=name or -d datadog.service=name)
//! 2. Detecting Laravel applications via `artisan` console and extracting app name from .env or config/app.php

use crate::procfs::Cmdline;
use crate::service_name::{DetectionContext, ServiceNameMetadata, ServiceNameSource};
use nom::{
    IResult,
    branch::alt,
    bytes::complete::{escaped_transform, tag, take_until, take_while, take_while1},
    character::complete::{anychar, char, multispace0, none_of},
    combinator::value,
};
use std::io::{BufRead, BufReader};
use std::path::{Path, PathBuf};

/// Extracts the PHP service name from command-line arguments.
///
/// Priority order:
/// 1. `datadog.service` flag (if present, regardless of other args)
/// 2. Laravel app name (if `artisan` command is detected)
/// 3. None (if nothing matches)
pub fn extract_name(cmdline: &Cmdline, ctx: &mut DetectionContext) -> Option<ServiceNameMetadata> {
    let mut artisan_path: Option<String> = None;
    let mut prev_arg_is_flag = false;

    let mut args = cmdline.args();

    // Skip the first arg (php executable)
    args.next()?;

    // Single pass: look for both datadog.service flag (priority 1) and artisan (priority 2)
    for arg in args {
        // Check for datadog.service flag (highest priority)
        if arg.contains("datadog.service=")
            && let Some((_prefix, value)) = arg.split_once("datadog.service=")
        {
            let service_name = value.trim();
            if !service_name.is_empty() {
                return Some(ServiceNameMetadata::new(
                    service_name,
                    ServiceNameSource::CommandLine,
                ));
            }
        }

        // If we already found artisan, skip the rest of the logic
        if artisan_path.is_some() {
            continue;
        }

        let has_flag_prefix = arg.starts_with('-');

        // If previous arg was a flag without =, skip this arg (it's the flag's value)
        if prev_arg_is_flag && !has_flag_prefix {
            prev_arg_is_flag = false;
            continue;
        }

        // If this arg is not a flag and wasn't preceded by a flag, check if it's artisan
        if !prev_arg_is_flag
            && !has_flag_prefix
            && let Some(base_path) = Path::new(arg).file_name().and_then(|f| f.to_str())
            && base_path == "artisan"
        {
            artisan_path = Some(arg.to_string());
        }

        // Update flag state for next iteration
        let includes_assignment = arg.contains('=');
        prev_arg_is_flag = has_flag_prefix && !includes_assignment;
    }

    // If we found artisan (and no datadog.service flag), return Laravel service
    if let Some(artisan_path) = artisan_path {
        let app_name = get_laravel_app_name(&artisan_path, ctx);
        return Some(ServiceNameMetadata::new(
            app_name,
            ServiceNameSource::Laravel,
        ));
    }

    None
}

// ============================================================================
// Laravel service name detection
// ============================================================================

/// Gets the Laravel app name from .env or config/app.php
///
/// Priority:
/// 1. .env file: DD_SERVICE > OTEL_SERVICE_NAME > APP_NAME
/// 2. config/app.php: parse PHP config for 'name' field
/// 3. Default: "laravel"
fn get_laravel_app_name(artisan_path: &str, ctx: &mut DetectionContext) -> String {
    let laravel_dir = Path::new(artisan_path)
        .parent()
        .map(|p| p.to_path_buf())
        .unwrap_or_else(|| PathBuf::from("."));

    // Try .env first
    if let Some(name) = get_laravel_app_name_from_env(&laravel_dir, ctx) {
        return name;
    }

    // Try config/app.php
    if let Some(name) = get_laravel_app_name_from_config(&laravel_dir, ctx) {
        return name;
    }

    // Default fallback
    "laravel".to_string()
}

/// Extracts app name from .env file
///
/// Priority: DD_SERVICE > OTEL_SERVICE_NAME > APP_NAME
fn get_laravel_app_name_from_env(laravel_dir: &Path, ctx: &mut DetectionContext) -> Option<String> {
    let env_file_path = laravel_dir.join(".env");

    // Try to resolve the path if it's relative
    let env_file_path = ctx
        .resolve_working_dir_relative_path(&env_file_path)
        .unwrap_or(env_file_path);

    let file = ctx.fs.open(&env_file_path).ok()?;
    let reader = file.verify(None).ok()?;
    let buf_reader = BufReader::new(reader);

    // Search for keys in a single pass through the file
    // Track all three values, then return in priority order
    let mut dd_service = None;
    let mut otel_service = None;
    let mut app_name = None;

    for line in buf_reader.lines().map_while(Result::ok) {
        let trimmed = line.trim();

        // Skip comments and empty lines
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }

        // Parse the line using nom
        if let Ok((_, (key, value))) = parse_env_line(trimmed) {
            // Store the value based on the key
            match key {
                "DD_SERVICE" if dd_service.is_none() => {
                    dd_service = Some(value);
                }
                "OTEL_SERVICE_NAME" if otel_service.is_none() => {
                    otel_service = Some(value);
                }
                "APP_NAME" if app_name.is_none() => {
                    app_name = Some(value);
                }
                _ => {}
            }

            // Early exit if we found the highest priority key
            if dd_service.is_some() {
                return dd_service;
            }
        }
    }

    // Return in priority order: DD_SERVICE > OTEL_SERVICE_NAME > APP_NAME
    dd_service.or(otel_service).or(app_name)
}

/// Extracts app name from config/app.php
///
/// Looks for patterns like:
/// - `'name' => env('APP_NAME', 'my-app-name')`
/// - `'name' => "my-app-name"`
fn get_laravel_app_name_from_config(
    laravel_dir: &Path,
    ctx: &mut DetectionContext,
) -> Option<String> {
    let config_file_path = laravel_dir.join("config").join("app.php");

    // Try to resolve the path if it's relative
    let config_file_path = ctx
        .resolve_working_dir_relative_path(&config_file_path)
        .unwrap_or(config_file_path);

    let file = ctx.fs.open(&config_file_path).ok()?;
    let reader = file.verify(None).ok()?;
    let buf_reader = BufReader::new(reader);

    // Parse the config file line by line looking for 'name' key
    for line in buf_reader.lines().map_while(Result::ok) {
        if let Some(value) = parse_laravel_config_name(&line) {
            return Some(value);
        }
    }

    None
}

/// Parses Laravel config/app.php to extract the 'name' value
///
/// Looks for patterns:
/// 1. env('APP_NAME', 'default-value') or env("APP_NAME", "default-value")
/// 2. 'name' => 'value' or "name" => "value"
///
/// Note: This expects 'name' at the start of the line. Multiple keys on the
/// same line (e.g., 'debug' => true, 'name' => 'app') are not supported, but
/// this case does not appear to be common in practice.
fn parse_laravel_config_name(contents: &str) -> Option<String> {
    parse_name_pattern(contents).ok().map(|(_, value)| value)
}

// ============================================================================
// Nom-based parsers for Laravel config/app.php
// ============================================================================

/// Parse the exact pattern: 'name' => value or "name" => value
fn parse_name_pattern(input: &str) -> IResult<&str, String> {
    let (input, _) = multispace0(input)?;
    let (input, quote) = alt((char('\''), char('"')))(input)?;
    let (input, _) = tag("name")(input)?;
    let (input, _) = char(quote)(input)?;
    let (input, _) = multispace0(input)?;
    let (input, _) = tag("=>")(input)?;
    let (input, _) = multispace0(input)?;

    alt((parse_env_value, parse_quoted_string))(input)
}

/// Parse env('KEY', 'default-value')
fn parse_env_value(input: &str) -> IResult<&str, String> {
    let (input, _) = tag("env")(input)?;
    let (input, _) = multispace0(input)?;
    let (input, _) = char('(')(input)?;
    let (input, _) = take_until(",")(input)?; // Skip the key part
    let (input, _) = char(',')(input)?;
    let (input, _) = multispace0(input)?;

    // Parse the default value (quoted string)
    parse_quoted_string(input)
}

/// Parse a quoted string with either single or double quotes, handling escape sequences
fn parse_quoted_string(input: &str) -> IResult<&str, String> {
    let (input, quote) = alt((char('\''), char('"')))(input)?;

    let (input, value) = if quote == '\'' {
        alt((
            escaped_transform(none_of("\\'"), '\\', anychar),
            value(String::new(), tag("")),
        ))(input)?
    } else {
        alt((
            escaped_transform(none_of("\\\""), '\\', anychar),
            value(String::new(), tag("")),
        ))(input)?
    };

    let (input, _) = char(quote)(input)?;

    Ok((input, value))
}

// ============================================================================
// Nom-based parsers for .env files
// ============================================================================

/// Parse a .env file line: KEY=VALUE or KEY = VALUE
/// Returns (key, value) tuple
fn parse_env_line(input: &str) -> IResult<&str, (&str, String)> {
    let (input, _) = multispace0(input)?;
    // Parse key (alphanumeric + underscore)
    let (input, key) = take_while1(|c: char| c.is_alphanumeric() || c == '_')(input)?;
    let (input, _) = multispace0(input)?;
    let (input, _) = char('=')(input)?;
    let (input, _) = multispace0(input)?;
    // Parse value (quoted or unquoted)
    let (input, value) = parse_env_value_string(input)?;

    Ok((input, (key, value)))
}

/// Parse an .env value (quoted or unquoted)
fn parse_env_value_string(input: &str) -> IResult<&str, String> {
    // Try quoted string first, then fall back to unquoted
    alt((parse_quoted_string, parse_unquoted_env_value))(input)
}

/// Parse an unquoted .env value (everything until end of line or comment)
fn parse_unquoted_env_value(input: &str) -> IResult<&str, String> {
    let (input, value) = take_while(|c: char| c != '#' && c != '\n' && c != '\r')(input)?;
    let value = value.trim();
    Ok((input, value.to_string()))
}

#[cfg(test)]
#[allow(clippy::expect_used, clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::fs::SubDirFs;
    use std::collections::HashMap;
    use std::fs;
    use std::io::Write;
    use tempfile::TempDir;

    fn test_ctx() -> (TempDir, HashMap<String, String>, SubDirFs) {
        let temp_dir = TempDir::new().expect("Failed to create temp dir");
        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        (temp_dir, envs, fs)
    }

    // ========================================================================
    // PHP detector tests (port of TestServiceNameFromCLI from Go)
    // ========================================================================

    #[test]
    fn test_should_return_laravel_for_artisan_commands() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "artisan", "serve"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "laravel",
                ServiceNameSource::Laravel
            ))
        );
    }

    #[test]
    fn test_should_return_service_name_for_php_d_datadog_service_inline() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "-ddatadog.service=service_name", "server.php"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "service_name",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn test_should_return_service_name_for_php_d_datadog_service_separate() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "-d", "datadog.service=service_name", "server.php"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "service_name",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn test_should_return_dd_service_when_both_present() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "-ddatadog.service=foo", "artisan", "serve"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "foo",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn test_artisan_command_with_x_flag() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "-x", "a", "artisan", "serve"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "laravel",
                ServiceNameSource::Laravel
            ))
        );
    }

    #[test]
    fn test_artisan_command_with_x_flag_and_assignment() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "-x=a", "artisan", "serve"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "laravel",
                ServiceNameSource::Laravel
            ))
        );
    }

    #[test]
    fn test_nothing_found() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline!["php", "server.php"];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(result, None);
    }

    #[test]
    fn test_empty_cmdline() {
        let (_temp_dir, envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let cmdline = cmdline![];

        let result = extract_name(&cmdline, &mut ctx);
        assert_eq!(result, None);
    }

    // ========================================================================
    // Laravel parser tests (port of TestGetLaravelAppNameFromEnv from Go)
    // ========================================================================

    #[test]
    fn test_get_laravel_app_name_from_env_with_app_name() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(&env_path, "APP_NAME=my-first-name\n").unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-first-name");
    }

    #[test]
    fn test_get_laravel_app_name_from_env_dd_service_priority() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(
            &env_path,
            "APP_NAME=my-first-name\nDD_SERVICE=my-dd-service\n",
        )
        .unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-dd-service");
    }

    #[test]
    fn test_get_laravel_app_name_from_env_otel_service_priority() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(
            &env_path,
            "APP_NAME=my-first-name\nOTEL_SERVICE_NAME=my-otel-service\n",
        )
        .unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-otel-service");
    }

    #[test]
    fn test_get_laravel_app_name_from_env_dd_over_otel() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(
            &env_path,
            "OTEL_SERVICE_NAME=my-otel-service\nDD_SERVICE=my-dd-service\n",
        )
        .unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-dd-service");
    }

    #[test]
    fn test_get_laravel_app_name_from_env_with_quotes() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(&env_path, "APP_NAME=\"my-quoted-name\"\n").unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-quoted-name");
    }

    #[test]
    fn test_get_laravel_app_name_from_env_with_single_quotes() {
        let temp_dir = TempDir::new().unwrap();
        let env_path = temp_dir.path().join(".env");
        fs::write(&env_path, "APP_NAME='my-quoted-name'\n").unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-quoted-name");
    }

    #[test]
    fn test_get_laravel_app_name_from_config_with_env_default() {
        let temp_dir = TempDir::new().unwrap();
        let config_dir = temp_dir.path().join("config");
        fs::create_dir(&config_dir).unwrap();
        let config_path = config_dir.join("app.php");
        let mut file = fs::File::create(&config_path).unwrap();
        writeln!(
            file,
            "<?php\nreturn [\n    'name' => env('APP_NAME', 'my-first-name'),\n];"
        )
        .unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-first-name");
    }

    #[test]
    fn test_get_laravel_app_name_from_config_direct_assignment() {
        let temp_dir = TempDir::new().unwrap();
        let config_dir = temp_dir.path().join("config");
        fs::create_dir(&config_dir).unwrap();
        let config_path = config_dir.join("app.php");
        let mut file = fs::File::create(&config_path).unwrap();
        writeln!(file, "<?php\nreturn [\n    'name' => 'my-first-name',\n];").unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-first-name");
    }

    #[test]
    fn test_get_laravel_app_name_from_config_with_double_quotes() {
        let temp_dir = TempDir::new().unwrap();
        let config_dir = temp_dir.path().join("config");
        fs::create_dir(&config_dir).unwrap();
        let config_path = config_dir.join("app.php");
        let mut file = fs::File::create(&config_path).unwrap();
        writeln!(
            file,
            r#"<?php
return [
    "name" => "my-first-name",
];"#
        )
        .unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "my-first-name");
    }

    #[test]
    fn test_defaults_to_laravel_when_nothing_found() {
        let temp_dir = TempDir::new().unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "laravel");
    }

    #[test]
    fn test_env_has_priority_over_config() {
        let temp_dir = TempDir::new().unwrap();

        // Create .env with APP_NAME
        let env_path = temp_dir.path().join(".env");
        fs::write(&env_path, "APP_NAME=env-name\n").unwrap();

        // Create config/app.php with different name
        let config_dir = temp_dir.path().join("config");
        fs::create_dir(&config_dir).unwrap();
        let config_path = config_dir.join("app.php");
        let mut file = fs::File::create(&config_path).unwrap();
        writeln!(file, "<?php\nreturn [\n    'name' => 'config-name',\n];").unwrap();

        let fs = SubDirFs::new(temp_dir.path()).expect("Failed to create SubDirFs");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let result = get_laravel_app_name("artisan", &mut ctx);
        assert_eq!(result, "env-name");
    }

    // ========================================================================
    // Edge case tests for parse_laravel_config_name
    // ========================================================================

    #[test]
    fn test_parse_laravel_config_name_with_valid_single_quotes() {
        let content = "'name' => 'my-app'";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("my-app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_with_valid_double_quotes() {
        let content = "\"name\" => \"my-app\"";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("my-app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_with_env_and_default() {
        let content = "'name' => env('APP_NAME', 'default-app')";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("default-app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_no_name_key() {
        let content = "'title' => 'My Title', 'version' => '1.0'";
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_name_not_as_key() {
        // 'name' appears but not as a quoted key
        let content = "name => 'my-app'"; // Missing quotes around name
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_similar_key() {
        // Keys like 'username' or 'firstname' shouldn't match
        let content = "'username' => 'john', 'firstname' => 'Jane'";
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_missing_arrow() {
        let content = "'name' 'my-app'"; // Missing =>
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_empty_value() {
        let content = "'name' => ''";
        assert_eq!(parse_laravel_config_name(content), Some("".to_string()));
    }

    #[test]
    fn test_parse_laravel_config_name_unclosed_quote() {
        let content = "'name' => 'my-app";
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_with_escaped_quotes() {
        let content = r#"'name' => 'my\'app'"#;
        assert_eq!(
            parse_laravel_config_name(content),
            Some("my'app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_env_without_default() {
        let content = "'name' => env('APP_NAME')";
        assert_eq!(parse_laravel_config_name(content), None);
    }

    #[test]
    fn test_parse_laravel_config_name_env_with_empty_default() {
        let content = "'name' => env('APP_NAME', '')";
        assert_eq!(parse_laravel_config_name(content), Some("".to_string()));
    }

    #[test]
    fn test_parse_laravel_config_name_multiple_name_keys() {
        // Should return the first one
        let content = "'name' => 'first-app', 'name' => 'second-app'";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("first-app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_with_whitespace() {
        let content = "  'name'  =>  'my-app'  ";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("my-app".to_string())
        );
    }

    #[test]
    fn test_parse_laravel_config_name_value_with_spaces() {
        let content = "'name' => 'my app with spaces'";
        assert_eq!(
            parse_laravel_config_name(content),
            Some("my app with spaces".to_string())
        );
    }
}
