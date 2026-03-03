// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#![allow(clippy::unwrap_used)]
#![allow(clippy::expect_used)]

use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process::Command;
use tempfile::{NamedTempFile, TempDir};

const SYSTEM_PROBE_LITE_BIN: &str = env!("CARGO_BIN_EXE_system-probe-lite");

fn mock_system_probe_path() -> PathBuf {
    // Bazel test: use the path provided via environment variable
    if let Ok(script_path) = std::env::var("MOCK_SYSTEM_PROBE") {
        return PathBuf::from(script_path);
    }

    // Cargo test: use CARGO_MANIFEST_DIR
    if let Ok(manifest_dir) = std::env::var("CARGO_MANIFEST_DIR") {
        return PathBuf::from(manifest_dir).join("testdata/fallback/mock-system-probe.sh");
    }

    panic!("Neither MOCK_SYSTEM_PROBE nor CARGO_MANIFEST_DIR is set");
}

#[test]
fn test_fallback_on_npm_enabled() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Run system-probe-lite with network tracer enabled using new -- syntax
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--pid=/var/run/test.pid")
        .arg("--debug")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    // Verify system-probe was called with correct args
    assert!(marker_file.exists(), "System-probe should have been called");
    let content = fs::read_to_string(&marker_file).unwrap();
    let expected_args = "run --pid=/var/run/test.pid --debug";
    assert!(
        content.contains(expected_args),
        "Expected arguments '{}', got: {}",
        expected_args,
        content
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains(
            "Falling back to system-probe: env var DD_SYSTEM_PROBE_NETWORK_ENABLED is set"
        ),
        "Expected fallback due to DD_SYSTEM_PROBE_NETWORK_ENABLED, got: {}",
        stdout
    );
}

#[test]
fn test_no_fallback_on_discovery_only() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Run system-probe-lite with only discovery enabled (should NOT fallback)
    // Note: --config must be after -- to be parsed as system-probe arg
    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    // Give it a moment to potentially call system-probe
    std::thread::sleep(std::time::Duration::from_millis(500));

    // Verify system-probe was NOT called
    assert!(
        !marker_file.exists(),
        "System-probe should NOT have been called"
    );

    // Clean up
    child.kill().ok();
    child.wait().expect("Failed to wait on system-probe-lite");
}

#[test]
fn test_config_file_only() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create config file with network enabled
    let mut config_file = NamedTempFile::new().unwrap();
    config_file
        .write_all(b"network_config:\n  enabled: true\n")
        .unwrap();
    config_file.flush().unwrap();

    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(marker_file.exists(), "Should fallback based on config file");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: YAML key network_config is active"),
        "Expected fallback due to network_config YAML key, got: {}",
        stdout
    );
}

#[test]
fn test_missing_fallback_binary() {
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg("/nonexistent/system-probe")
        .arg("run")
        .env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(!output.status.success(), "Should fail with missing binary");
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("does not exist") || stderr.contains("Failed"),
        "Should have error message about missing binary"
    );
}

#[test]
fn test_invalid_yaml_triggers_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create invalid YAML config
    let mut config_file = NamedTempFile::new().unwrap();
    config_file
        .write_all(b"invalid: yaml: content:\n  bad indentation")
        .unwrap();
    config_file.flush().unwrap();

    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(marker_file.exists(), "Should fallback on invalid YAML");
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Failed to load YAML config. Falling back to system-probe."),
        "Expected fallback due to invalid YAML, got: {}",
        stdout
    );
}

#[test]
fn test_unknown_yaml_key_triggers_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create config with unknown key
    let mut config_file = NamedTempFile::new().unwrap();
    config_file
        .write_all(b"unknown_module:\n  enabled: true\n")
        .unwrap();
    config_file.flush().unwrap();

    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Unknown YAML key should trigger fallback"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: YAML key unknown_module is active"),
        "Expected fallback due to unknown_module YAML key, got: {}",
        stdout
    );
}

#[test]
fn test_discovery_disabled_exits_cleanly() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Discovery explicitly disabled should exit cleanly (not call system-probe)
    // Note: --config must be after -- to be parsed as system-probe arg
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_ENABLED", "false")
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        !marker_file.exists(),
        "Discovery disabled should NOT call system-probe"
    );
    assert!(
        output.status.success(),
        "Discovery disabled should exit cleanly with success"
    );
}

#[test]
fn test_discovery_enabled_with_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Both discovery and DD_RUNTIME_SECURITY_CONFIG_ENABLED should trigger fallback
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .env("DD_RUNTIME_SECURITY_CONFIG_ENABLED", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Discovery + Runtime Security Config Enabled should trigger fallback"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains(
            "Falling back to system-probe: env var DD_RUNTIME_SECURITY_CONFIG_ENABLED is set"
        ),
        "Expected fallback due to DD_RUNTIME_SECURITY_CONFIG_ENABLED, got: {}",
        stdout
    );
}

// Killswitch integration tests
#[test]
fn test_killswitch_disabled_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Killswitch disabled should trigger fallback even with discovery enabled
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "false")
        .env("DD_DISCOVERY_ENABLED", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Killswitch disabled should trigger fallback to system-probe"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: sd-agent killswitch is not enabled"),
        "Expected fallback due to killswitch disabled, got: {}",
        stdout
    );
}

#[test]
fn test_killswitch_not_set_defaults_to_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Killswitch not set should default to fallback (safe default)
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_ENABLED", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Killswitch not set should default to fallback (safe default)"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: sd-agent killswitch is not enabled"),
        "Expected fallback due to killswitch not set, got: {}",
        stdout
    );
}

#[test]
fn test_killswitch_enabled_runs_system_probe_lite() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Killswitch enabled should allow system-probe-lite to run
    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .spawn()
        .expect("Failed to execute system-probe-lite");

    // Give it time to start
    std::thread::sleep(std::time::Duration::from_millis(500));

    // Terminate the process
    #[cfg(unix)]
    {
        use nix::sys::signal::{Signal, kill};
        use nix::unistd::Pid;
        kill(Pid::from_raw(child.id() as i32), Signal::SIGTERM).unwrap();
    }

    let _status = child.wait().expect("Failed to wait for child process");

    assert!(
        !marker_file.exists(),
        "Killswitch enabled should NOT trigger fallback - system-probe-lite should run"
    );
}

#[test]
fn test_killswitch_yaml_config() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create YAML config with killswitch disabled
    let mut config_file = NamedTempFile::new().unwrap();
    config_file
        .write_all(
            b"discovery:
  use_system_probe_lite: false
  enabled: true
",
        )
        .unwrap();
    config_file.flush().unwrap();

    // YAML with killswitch disabled should trigger fallback
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "YAML with killswitch disabled should trigger fallback"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: sd-agent killswitch is not enabled"),
        "Expected fallback due to killswitch disabled in YAML, got: {}",
        stdout
    );
}

#[test]
fn test_killswitch_env_overrides_yaml_enabled() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create YAML config with killswitch enabled
    let mut config_file = NamedTempFile::new().unwrap();
    config_file
        .write_all(
            b"discovery:
  use_system_probe_lite: true
  enabled: true
",
        )
        .unwrap();
    config_file.flush().unwrap();

    // Env var (false) should override YAML (true)
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "false")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Env var should override YAML - fallback should happen"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains("Falling back to system-probe: sd-agent killswitch is not enabled"),
        "Expected fallback due to killswitch env var override, got: {}",
        stdout
    );
}

#[test]
fn test_env_var_false_no_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Env var set to "false" should NOT trigger fallback
    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "false")
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    std::thread::sleep(std::time::Duration::from_millis(500));

    assert!(
        !marker_file.exists(),
        "Env var set to 'false' should NOT trigger fallback"
    );

    child.kill().ok();
    child.wait().expect("Failed to wait on system-probe-lite");
}

#[test]
fn test_env_var_zero_no_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Env var set to "0" should NOT trigger fallback
    let mut child = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "0")
        .spawn()
        .expect("Failed to spawn system-probe-lite");

    std::thread::sleep(std::time::Duration::from_millis(500));

    assert!(
        !marker_file.exists(),
        "Env var set to '0' should NOT trigger fallback"
    );

    child.kill().ok();
    child.wait().expect("Failed to wait on system-probe-lite");
}

#[test]
fn test_env_var_non_boolean_triggers_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Empty config file to avoid picking up /etc/datadog-agent/system-probe.yaml
    let config_file = NamedTempFile::new().unwrap();

    // Env var set to a non-boolean value should trigger fallback (safety net)
    let output = Command::new(SYSTEM_PROBE_LITE_BIN)
        .env_clear()
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_SYSTEM_PROBE_NETWORK_ENABLED", "maybe")
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .output()
        .expect("Failed to execute system-probe-lite");

    assert!(
        marker_file.exists(),
        "Env var with non-boolean value should trigger fallback as safety net"
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(
        stdout.contains(
            "Falling back to system-probe: env var DD_SYSTEM_PROBE_NETWORK_ENABLED is set"
        ),
        "Expected fallback due to non-boolean DD_SYSTEM_PROBE_NETWORK_ENABLED, got: {}",
        stdout
    );
}
