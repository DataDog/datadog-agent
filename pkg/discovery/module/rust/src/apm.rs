// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;

use crate::language::Language;
use crate::procfs::{self, Cmdline};

/// Detects whether a service has APM instrumentation using language-specific detection.
///
/// This is only used if tracer metadata is not available.
pub fn detect(
    lang: &Language,
    pid: i32,
    cmdline: &Cmdline,
    envs: &HashMap<String, String>,
) -> bool {
    // Check language-specific detection
    match lang {
        Language::Python => detect_python(pid),
        Language::Java => detect_java(cmdline, envs),
        Language::DotNet => detect_dotnet(pid, envs),
        _ => false,
    }
}

/// Detects Python APM instrumentation by scanning /proc/PID/maps for ddtrace library.
///
/// The Python ddtrace package includes native libraries (.so files) that appear
/// in the process memory map when loaded.
fn detect_python(pid: i32) -> bool {
    match procfs::maps::get_reader_for_pid(pid) {
        Ok(reader) => detect_python_from_reader(reader),
        Err(_) => false,
    }
}

/// Detects Python APM instrumentation by scanning a maps reader for ddtrace library.
/// Since the ddtrace package uses native libraries, the paths of these libraries
/// will show up in /proc/$PID/maps.
///
/// It looks for the "/ddtrace/" part of the path. It doesn not look for the
/// "/site-packages/" part since some environments (such as pyinstaller) may not
/// use that exact name.
///
/// For example:
/// 7aef453fc000-7aef453ff000 rw-p 0004c000 fc:06 7895473  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/_encoding.cpython-310-x86_64-linux-gnu.so
/// 7aef45400000-7aef45459000 r--p 00000000 fc:06 7895588  /home/foo/.local/lib/python3.10/site-packages/ddtrace/internal/datadog/profiling/libdd_wrapper.so
fn detect_python_from_reader<R: std::io::BufRead>(reader: R) -> bool {
    reader.lines().any(|line| match line {
        Ok(line) => line.contains("/ddtrace/"),
        Err(_) => false,
    })
}

/// Detects Java APM instrumentation by checking command-line arguments and environment variables.
///
/// Java APM instrumentation is typically done via the -javaagent flag or environment variables
/// that inject the agent at JVM startup.
fn detect_java(cmdline: &Cmdline, envs: &HashMap<String, String>) -> bool {
    // Check command-line arguments for -javaagent flag
    for arg in cmdline.args() {
        if is_datadog_java_agent(arg) {
            return true;
        }
    }

    // Check Java environment variables for agent configuration
    const JAVA_ENV_VARS: &[&str] = &[
        "JAVA_TOOL_OPTIONS",
        "_JAVA_OPTIONS",
        "JDK_JAVA_OPTIONS",
        "JAVA_OPTIONS",
        "CATALINA_OPTS",
        "JDPA_OPTS",
    ];

    for env_var in JAVA_ENV_VARS {
        if let Some(value) = envs.get(*env_var)
            && is_datadog_java_agent(value)
        {
            return true;
        }
    }

    false
}

/// Checks if a string contains a Datadog Java agent configuration.
///
/// Matches patterns like: -javaagent:/path/to/dd-java-agent.jar
fn is_datadog_java_agent(s: &str) -> bool {
    if !s.starts_with("-javaagent:") {
        return false;
    }

    if !s.ends_with(".jar") {
        return false;
    }

    // Check if it contains one of the Datadog agent identifiers
    s.contains("datadog") || s.contains("dd-java-agent") || s.contains("dd-trace-agent")
}

/// Detects .NET APM instrumentation via environment variable or maps scan.
///
/// The primary check is for the environment variables which enables .NET
/// profiling. This is required for auto-instrumentation, and besides that custom
/// instrumentation using version 3.0.0 or later of Datadog.Trace requires
/// auto-instrumentation. It is also set if some third-party
/// profiling/instrumentation is active.
///
/// The secondary check is to detect cases where an older version of
/// Datadog.Trace is used for manual instrumentation without enabling
/// auto-instrumentation. For this, we check for the presence of the DLL in the
/// maps file. Note that this does not work for single-file deployments.
///
/// 785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267 /home/foo/.../publish/Datadog.Trace.dll
fn detect_dotnet(pid: i32, envs: &HashMap<String, String>) -> bool {
    // Check CORECLR_ENABLE_PROFILING environment variable
    if let Some(value) = envs.get("CORECLR_ENABLE_PROFILING")
        && value == "1"
    {
        return true;
    }

    // Fallback: scan /proc/PID/maps for Datadog.Trace.dll
    match procfs::maps::get_reader_for_pid(pid) {
        Ok(reader) => detect_dotnet_from_reader(reader),
        Err(_) => false,
    }
}

/// Detects .NET APM instrumentation by scanning a maps reader for Datadog.Trace.dll.
fn detect_dotnet_from_reader<R: std::io::BufRead>(reader: R) -> bool {
    reader.lines().any(|line| match line {
        Ok(line) => line.ends_with("Datadog.Trace.dll"),
        Err(_) => false,
    })
}

#[cfg(test)]
#[allow(clippy::expect_used, clippy::undocumented_unsafe_blocks)]
mod tests {
    use crate::cmdline;

    use super::*;

    #[test]
    fn test_detect_without_instrumentation() {
        let cmdline = cmdline![];
        let envs = HashMap::new();

        // Unknown language should return false
        let result = detect(&Language::Unknown, 1, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_detect_java_with_javaagent() {
        let cmdline = cmdline![
            "java",
            "-javaagent:/opt/datadog/dd-java-agent.jar",
            "-jar",
            "app.jar",
        ];
        let envs = HashMap::new();

        let result = detect(&Language::Java, 1, &cmdline, &envs);
        assert!(result);
    }

    #[test]
    fn test_detect_java_with_env_var() {
        let cmdline = cmdline!["java", "-jar", "app.jar"];
        let mut envs = HashMap::new();
        envs.insert(
            "JAVA_TOOL_OPTIONS".to_string(),
            "-javaagent:/opt/dd-java-agent.jar".to_string(),
        );

        let result = detect(&Language::Java, 1, &cmdline, &envs);
        assert!(result);
    }

    #[test]
    fn test_detect_java_without_agent() {
        let cmdline = cmdline!["java", "-jar", "app.jar"];
        let envs = HashMap::new();

        let result = detect(&Language::Java, 1, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_java_agent_pattern_matching() {
        // Valid patterns
        assert!(is_datadog_java_agent("-javaagent:/opt/datadog.jar"));
        assert!(is_datadog_java_agent("-javaagent:/opt/dd-java-agent.jar"));
        assert!(is_datadog_java_agent("-javaagent:/opt/dd-trace-agent.jar"));
        assert!(is_datadog_java_agent("-javaagent:dd-java-agent-1.0.0.jar"));
        assert!(is_datadog_java_agent(
            "-javaagent:/path/to/datadog-agent-v2.jar"
        ));

        // Invalid patterns
        assert!(!is_datadog_java_agent("-javaagent:/opt/other-agent.jar"));
        assert!(!is_datadog_java_agent("-jar app.jar"));
        assert!(!is_datadog_java_agent("datadog.jar"));
    }

    #[test]
    fn test_detect_dotnet_with_profiling_enabled() {
        let cmdline = cmdline![];
        let mut envs = HashMap::new();
        envs.insert("CORECLR_ENABLE_PROFILING".to_string(), "1".to_string());

        let result = detect(&Language::DotNet, 9999999, &cmdline, &envs);
        assert!(result);
    }

    #[test]
    fn test_detect_dotnet_with_profiling_disabled() {
        let cmdline = cmdline![];
        let mut envs = HashMap::new();
        envs.insert("CORECLR_ENABLE_PROFILING".to_string(), "0".to_string());

        let result = detect(&Language::DotNet, 9999999, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_detect_dotnet_without_profiling() {
        let cmdline = cmdline![];
        let envs = HashMap::new();

        // Should attempt to scan maps, but will fail for nonexistent PID
        let result = detect(&Language::DotNet, 9999999, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_detect_python_no_instrumentation() {
        // Test with nonexistent PID
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let result = detect(&Language::Python, 9999999, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_detect_python_from_reader_empty_maps() {
        use std::io::Cursor;
        let maps = "";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!detect_python_from_reader(reader));
    }

    #[test]
    fn test_detect_python_from_reader_not_in_maps() {
        use std::io::Cursor;
        let maps = "79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd479000-79f6cd47a000 r-xp 00001000 fc:06 5507018                    /home/foo/.local/lib/python3.10/site-packages/ddtrace_fake/md.cpython-310-x86_64-linux-gnu.so";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!detect_python_from_reader(reader));
    }

    #[test]
    fn test_detect_python_from_reader_in_maps() {
        use std::io::Cursor;
        let maps = "79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd438000-79f6cd441000 r-xp 00004000 fc:06 7895596                    /home/foo/.local/lib/python3.10/site-packages-internal/ddtrace/internal/datadog/profiling/crashtracker/_crashtracker.cpython-310-x86_64-linux-gnu.so";
        let reader = Cursor::new(maps.as_bytes());
        assert!(detect_python_from_reader(reader));
    }

    #[test]
    fn test_detect_dotnet_from_reader_no_env_no_maps() {
        use std::io::Cursor;
        let maps = "";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!detect_dotnet_from_reader(reader));
    }

    #[test]
    fn test_detect_dotnet_from_reader_not_in_maps() {
        use std::io::Cursor;
        let maps = "785c8ab24000-785c8ab2c000 r--s 00000000 fc:06 12762114                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/System.Diagnostics.StackTrace.dll";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!detect_dotnet_from_reader(reader));
    }

    #[test]
    fn test_detect_dotnet_from_reader_in_maps() {
        use std::io::Cursor;
        let maps = "785c8a400000-785c8aaeb000 r--s 00000000 fc:06 12762267                   /home/foo/hello/bin/release/net8.0/linux-x64/publish/Datadog.Trace.dll";
        let reader = Cursor::new(maps.as_bytes());
        assert!(detect_dotnet_from_reader(reader));
    }

    #[test]
    fn test_detect_dotnet_env_profiling_enabled() {
        let cmdline = cmdline![];
        let mut envs = HashMap::new();
        envs.insert("CORECLR_ENABLE_PROFILING".to_string(), "1".to_string());

        // Should return true based on env var alone (maps check would fail with nonexistent PID)
        let result = detect(&Language::DotNet, 9999999, &cmdline, &envs);
        assert!(result);
    }

    #[test]
    fn test_detect_dotnet_env_profiling_disabled() {
        let cmdline = cmdline![];
        let mut envs = HashMap::new();
        envs.insert("CORECLR_ENABLE_PROFILING".to_string(), "0".to_string());

        // Should not detect as instrumented if value is not "1"
        // (would need to scan maps, but that would fail for nonexistent PID)
        let result = detect(&Language::DotNet, 9999999, &cmdline, &envs);
        assert!(!result);
    }

    #[test]
    fn test_detect_python_integration() {
        use std::fs::File;

        use memmap2::Mmap;

        let current_pid = std::process::id().cast_signed();
        let cmdline = cmdline![];
        let envs = HashMap::new();

        // Negative test: current process should NOT be detected as Python with APM initially
        let result = detect(&Language::Python, current_pid, &cmdline, &envs);
        assert!(
            !result,
            "Process should not have Python APM before mmapping ddtrace library"
        );

        // Create path to test ddtrace library
        let lib_path = crate::test_utils::testdata_path()
            .join("ddtrace/_crashtracker.cpython-310-x86_64-linux-gnu.so");

        // Memory-map the ddtrace library file
        let file = File::open(&lib_path).expect("Failed to open ddtrace test library");
        let _mmap = unsafe { Mmap::map(&file).expect("Failed to mmap ddtrace library") };

        // Positive test: current process SHOULD be detected as having Python APM after mmapping
        let result = detect(&Language::Python, current_pid, &cmdline, &envs);
        assert!(
            result,
            "Process should have Python APM after mmapping ddtrace library"
        );

        // Keep mmap alive until the end of the test
        drop(_mmap);
    }

    #[test]
    fn test_detect_dotnet_integration() {
        use std::fs::File;

        use memmap2::Mmap;

        let current_pid = std::process::id().cast_signed();
        let cmdline = cmdline![];
        let envs = HashMap::new(); // No env vars set

        // Negative test: current process should NOT be detected as .NET with APM initially
        let result = detect(&Language::DotNet, current_pid, &cmdline, &envs);
        assert!(
            !result,
            "Process should not have .NET APM before mmapping Datadog.Trace.dll"
        );

        // Create path to test Datadog.Trace.dll
        let dll_path = crate::test_utils::testdata_path().join("Datadog.Trace.dll");

        // Memory-map the Datadog.Trace.dll file
        let file = File::open(&dll_path).expect("Failed to open Datadog.Trace.dll test file");
        let _mmap = unsafe { Mmap::map(&file).expect("Failed to mmap Datadog.Trace.dll") };

        // Positive test: current process SHOULD be detected as having .NET APM after mmapping
        let result = detect(&Language::DotNet, current_pid, &cmdline, &envs);
        assert!(
            result,
            "Process should have .NET APM after mmapping Datadog.Trace.dll"
        );

        // Keep mmap alive until the end of the test
        drop(_mmap);
    }
}
