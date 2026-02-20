// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;
use std::fs;
use std::io::{BufRead, BufReader};

use phf::phf_set;

use crate::procfs;

/// Target environment variables to collect from processes.
/// These are used for APM detection and service discovery.
/// Using phf::Set for O(1) compile-time perfect hash lookup.
static TARGET_ENV_VARS: phf::Set<&'static str> = phf_set! {
    "PWD",
    "DD_SERVICE",
    "DD_ENV",
    "DD_VERSION",
    "DD_TAGS",
    "GUNICORN_CMD_ARGS",
    "WSGI_APP",
    "CORECLR_ENABLE_PROFILING",
    "CATALINA_OPTS",
    "JAVA_TOOL_OPTIONS",
    "_JAVA_OPTIONS",
    "JDK_JAVA_OPTIONS",
    "JAVA_OPTIONS",
    "JDPA_OPTS",
    "SPRING_APPLICATION_NAME",
    "SPRING_CONFIG_LOCATIONS",
    "SPRING_CONFIG_NAME",
    "SPRING_PROFILES_ACTIVE",
};

/// Reads and filters environment variables from a process.
///
/// Reads `/proc/PID/environ` incrementally and collects only the target
/// environment variables defined in `TARGET_ENV_VARS`.
pub fn get_target_envs(pid: i32) -> Result<HashMap<String, String>, std::io::Error> {
    let path = procfs::root_path().join(pid.to_string()).join("environ");

    let file = fs::File::open(&path)?;
    let reader = BufReader::new(file);
    let mut env_vars = HashMap::new();

    // Read null-delimited environment variables incrementally
    for entry_result in reader.split(b'\0') {
        let bytes = entry_result?;

        if bytes.is_empty() {
            continue;
        }

        // Convert bytes to string (environ should be UTF-8, but use lossy conversion just in case)
        let entry = String::from_utf8_lossy(&bytes);

        if let Some((key, value)) = entry.split_once('=')
            && TARGET_ENV_VARS.contains(key)
        {
            env_vars.insert(key.to_string(), value.to_string());
        }
    }

    Ok(env_vars)
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used, clippy::print_stdout)]
mod tests {
    use super::*;
    use std::thread::sleep;
    use std::time::{Duration, Instant};

    /// Asserts that a condition becomes true within a timeout period.
    /// Checks the condition repeatedly at the specified tick interval.
    /// Similar to testify's `require.EventuallyWithT` in Go.
    fn assert_eventually<F>(timeout: Duration, tick: Duration, condition: F)
    where
        F: Fn() -> bool,
    {
        let start = Instant::now();
        loop {
            if condition() {
                return; // Condition met, success
            }
            if start.elapsed() >= timeout {
                // Timeout reached, perform the check one last time to get the failure message
                assert!(condition(), "Condition never satisfied within the timeout");
                return;
            }
            sleep(tick);
        }
    }

    #[test]
    fn test_get_target_envs_nonexistent_pid() {
        // Test with a PID that doesn't exist
        let result = get_target_envs(9999999);
        assert!(result.is_err());
    }

    #[test]
    fn test_get_target_envs_self() {
        // Test reading our own environment variables
        // This should succeed unless we don't have permission to read our own /proc/self/environ
        let result = get_target_envs(std::process::id().cast_signed());
        assert!(result.is_ok());

        let env_vars = result.unwrap();
        assert!(!env_vars.is_empty());
    }

    #[test]
    fn test_target_envs() {
        use std::process::Command;

        // Spawn a test process with all target env vars set to distinct values
        let mut cmd = Command::new("sleep");
        cmd.arg("1000");

        // Set all target environment variables to distinct values
        for var in TARGET_ENV_VARS.iter() {
            cmd.env(var, var.to_string() + "-value");
        }

        // Add some non-target variables that should be filtered out
        cmd.env("HOME", "/home/test");
        cmd.env("PATH", "/usr/bin");
        cmd.env("SHELL", "/bin/bash");

        let mut child = cmd.spawn().expect("Failed to spawn test process");
        let pid = child.id().cast_signed();

        // Wait for process to be fully initialized and environ to be readable
        assert_eventually(Duration::from_secs(5), Duration::from_millis(10), || {
            get_target_envs(pid)
                .map(|vars| !vars.is_empty())
                .unwrap_or(false)
        });

        // Read the environment variables
        let vars = get_target_envs(pid).expect("Failed to read target envs");
        println!("vars: {:#?}", vars);

        // Assert all target variables are present with correct values
        for var in TARGET_ENV_VARS.iter() {
            assert_eq!(vars.get(*var), Some(&(var.to_string() + "-value")));
        }

        // Assert non-target variables are NOT present in the filtered results
        assert_eq!(vars.get("HOME"), None, "HOME should be filtered out");
        assert_eq!(vars.get("PATH"), None, "PATH should be filtered out");
        assert_eq!(vars.get("SHELL"), None, "SHELL should be filtered out");

        // Cleanup: kill the spawned process
        let _ = child.kill();
        let _ = child.wait();
    }
}
