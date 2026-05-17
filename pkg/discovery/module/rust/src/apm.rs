// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;

use crate::language::Language;
use crate::procfs::Cmdline;
use crate::procfs::maps::MapsInfo;

/// Detects whether a service has APM instrumentation using language-specific detection.
///
/// This is only used if tracer metadata is not available.
pub fn detect(
    lang: Option<&Language>,
    cmdline: &Cmdline,
    envs: &HashMap<String, String>,
    maps_info: &MapsInfo,
) -> bool {
    match lang {
        Some(Language::Python) => maps_info.has_ddtrace,
        Some(Language::Java) => detect_java(cmdline, envs),
        Some(Language::DotNet) => detect_dotnet(envs, maps_info),
        _ => false,
    }
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

/// Detects .NET APM instrumentation via environment variable or pre-scanned maps info.
///
/// The primary check is for the environment variables which enables .NET
/// profiling. This is required for auto-instrumentation, and besides that custom
/// instrumentation using version 3.0.0 or later of Datadog.Trace requires
/// auto-instrumentation. It is also set if some third-party
/// profiling/instrumentation is active.
///
/// The secondary check uses the pre-scanned maps info to detect cases where an
/// older version of Datadog.Trace is used for manual instrumentation without
/// enabling auto-instrumentation.
fn detect_dotnet(envs: &HashMap<String, String>, maps_info: &MapsInfo) -> bool {
    // Check CORECLR_ENABLE_PROFILING environment variable
    if let Some(value) = envs.get("CORECLR_ENABLE_PROFILING")
        && value == "1"
    {
        return true;
    }

    maps_info.has_datadog_trace_dll
}

#[cfg(test)]
#[allow(
    clippy::expect_used,
    clippy::unwrap_used,
    clippy::undocumented_unsafe_blocks
)]
mod tests {
    use crate::cmdline;

    use super::*;

    #[test]
    fn test_detect_without_instrumentation() {
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let maps_info = MapsInfo::default();

        // Unknown language should return false
        let result = detect(None, &cmdline, &envs, &maps_info);
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
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::Java), &cmdline, &envs, &maps_info);
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
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::Java), &cmdline, &envs, &maps_info);
        assert!(result);
    }

    #[test]
    fn test_detect_java_without_agent() {
        let cmdline = cmdline!["java", "-jar", "app.jar"];
        let envs = HashMap::new();
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::Java), &cmdline, &envs, &maps_info);
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
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::DotNet), &cmdline, &envs, &maps_info);
        assert!(result);
    }

    #[test]
    fn test_detect_dotnet_with_profiling_disabled() {
        let cmdline = cmdline![];
        let mut envs = HashMap::new();
        envs.insert("CORECLR_ENABLE_PROFILING".to_string(), "0".to_string());
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::DotNet), &cmdline, &envs, &maps_info);
        assert!(!result);
    }

    #[test]
    fn test_detect_dotnet_without_profiling() {
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::DotNet), &cmdline, &envs, &maps_info);
        assert!(!result);
    }

    #[test]
    fn test_detect_dotnet_with_datadog_trace_dll_in_maps() {
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let maps_info = MapsInfo {
            has_datadog_trace_dll: true,
            ..Default::default()
        };

        let result = detect(Some(&Language::DotNet), &cmdline, &envs, &maps_info);
        assert!(result);
    }

    #[test]
    fn test_detect_python_no_instrumentation() {
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let maps_info = MapsInfo::default();

        let result = detect(Some(&Language::Python), &cmdline, &envs, &maps_info);
        assert!(!result);
    }

    #[test]
    fn test_detect_python_with_ddtrace_in_maps() {
        let cmdline = cmdline![];
        let envs = HashMap::new();
        let maps_info = MapsInfo {
            has_ddtrace: true,
            ..Default::default()
        };

        let result = detect(Some(&Language::Python), &cmdline, &envs, &maps_info);
        assert!(result);
    }

    #[test]
    fn test_detect_python_integration() {
        use std::fs::File;

        use memmap2::Mmap;

        let current_pid = std::process::id().cast_signed();
        let cmdline = cmdline![];
        let envs = HashMap::new();

        // Read maps info for current process
        let maps_info = crate::procfs::maps::read_maps_info(current_pid).unwrap();

        // Negative test: current process should NOT have ddtrace
        assert!(
            !maps_info.has_ddtrace,
            "Process should not have ddtrace before mmapping ddtrace library"
        );

        // Create path to test ddtrace library
        let lib_path = crate::test_utils::testdata_path()
            .join("ddtrace/_crashtracker.cpython-310-x86_64-linux-gnu.so");

        // Memory-map the ddtrace library file
        let file = File::open(&lib_path).expect("Failed to open ddtrace test library");
        let _mmap = unsafe { Mmap::map(&file).expect("Failed to mmap ddtrace library") };

        // Re-read maps info after mmapping
        let maps_info = crate::procfs::maps::read_maps_info(current_pid).unwrap();

        // Positive test: current process SHOULD have ddtrace after mmapping
        let result = detect(Some(&Language::Python), &cmdline, &envs, &maps_info);
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
        let envs = HashMap::new();

        // Read maps info for current process
        let maps_info = crate::procfs::maps::read_maps_info(current_pid).unwrap();

        // Negative test: current process should NOT have Datadog.Trace.dll
        assert!(
            !maps_info.has_datadog_trace_dll,
            "Process should not have .NET APM before mmapping Datadog.Trace.dll"
        );

        // Create path to test Datadog.Trace.dll
        let dll_path = crate::test_utils::testdata_path().join("Datadog.Trace.dll");

        // Memory-map the Datadog.Trace.dll file
        let file = File::open(&dll_path).expect("Failed to open Datadog.Trace.dll test file");
        let _mmap = unsafe { Mmap::map(&file).expect("Failed to mmap Datadog.Trace.dll") };

        // Re-read maps info after mmapping
        let maps_info = crate::procfs::maps::read_maps_info(current_pid).unwrap();

        // Positive test: current process SHOULD have Datadog.Trace.dll after mmapping
        let result = detect(Some(&Language::DotNet), &cmdline, &envs, &maps_info);
        assert!(
            result,
            "Process should have .NET APM after mmapping Datadog.Trace.dll"
        );

        // Keep mmap alive until the end of the test
        drop(_mmap);
    }
}
