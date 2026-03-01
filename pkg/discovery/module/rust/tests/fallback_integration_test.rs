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

const SD_AGENT_BIN: &str = env!("CARGO_BIN_EXE_sd-agent");

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

    // Run sd-agent with network tracer enabled using new -- syntax
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--pid=/var/run/test.pid")
        .arg("--debug")
        .env("DD_NETWORK_CONFIG_ENABLED", "true")
        .output()
        .expect("Failed to execute sd-agent");

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
}

#[test]
fn test_no_fallback_on_discovery_only() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create empty config file to avoid picking up system config at /etc/datadog-agent/system-probe.yaml
    let mut config_file = NamedTempFile::new().unwrap();
    config_file.write_all(b"").unwrap();
    config_file.flush().unwrap();

    // Run sd-agent with only discovery enabled (should NOT fallback)
    // Note: --config must be after -- to be parsed as system-probe arg
    let mut child = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .spawn()
        .expect("Failed to spawn sd-agent");

    // Give it a moment to potentially call system-probe
    std::thread::sleep(std::time::Duration::from_millis(500));

    // Verify system-probe was NOT called
    assert!(
        !marker_file.exists(),
        "System-probe should NOT have been called"
    );

    // Clean up
    child.kill().ok();
    child.wait().expect("Failed to wait on sd-agent");
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

    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .output()
        .expect("Failed to execute sd-agent");

    assert!(marker_file.exists(), "Should fallback based on config file");
}

#[test]
fn test_missing_fallback_binary() {
    let output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg("/nonexistent/system-probe")
        .arg("run")
        .env("DD_NETWORK_CONFIG_ENABLED", "true")
        .output()
        .expect("Failed to execute sd-agent");

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

    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .output()
        .expect("Failed to execute sd-agent");

    assert!(marker_file.exists(), "Should fallback on invalid YAML");
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

    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Unknown YAML key should trigger fallback"
    );
}

#[test]
fn test_discovery_disabled_exits_cleanly() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create empty config file to avoid picking up system config at /etc/datadog-agent/system-probe.yaml
    let mut config_file = NamedTempFile::new().unwrap();
    config_file.write_all(b"").unwrap();
    config_file.flush().unwrap();

    // Discovery explicitly disabled should exit cleanly (not call system-probe)
    // Note: --config must be after -- to be parsed as system-probe arg
    let output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg("--config")
        .arg(config_file.path())
        .env("DD_DISCOVERY_ENABLED", "false")
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .output()
        .expect("Failed to execute sd-agent");

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

    // Both discovery and DD_SERVICE should trigger fallback
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_RUNTIME_SECURITY_CONFIG_ENABLED", "true")
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Discovery + Runtime Security Config Enabled should trigger fallback"
    );
}

// Killswitch integration tests
#[test]
fn test_killswitch_disabled_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Killswitch disabled should trigger fallback even with discovery enabled
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .env("DD_DISCOVERY_USE_SD_AGENT", "false")
        .env("DD_DISCOVERY_ENABLED", "true")
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Killswitch disabled should trigger fallback to system-probe"
    );
}

#[test]
fn test_killswitch_not_set_defaults_to_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Killswitch not set should default to fallback (safe default)
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .env("DD_DISCOVERY_ENABLED", "true")
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Killswitch not set should default to fallback (safe default)"
    );
}

#[test]
fn test_killswitch_enabled_runs_sd_agent() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create empty config file to avoid picking up system config
    let mut config_file = NamedTempFile::new().unwrap();
    config_file.write_all(b"").unwrap();
    config_file.flush().unwrap();

    // Killswitch enabled should allow sd-agent to run
    let mut child = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .spawn()
        .expect("Failed to execute sd-agent");

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
        "Killswitch enabled should NOT trigger fallback - sd-agent should run"
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
  use_sd_agent: false
  enabled: true
",
        )
        .unwrap();
    config_file.flush().unwrap();

    // YAML with killswitch disabled should trigger fallback
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "YAML with killswitch disabled should trigger fallback"
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
  use_sd_agent: true
  enabled: true
",
        )
        .unwrap();
    config_file.flush().unwrap();

    // Env var (false) should override YAML (true)
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SD_AGENT", "false")
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Env var should override YAML - fallback should happen"
    );
}

#[test]
fn test_env_var_false_no_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create empty config file
    let mut config_file = NamedTempFile::new().unwrap();
    config_file.write_all(b"").unwrap();
    config_file.flush().unwrap();

    // Env var set to "false" should NOT trigger fallback
    let mut child = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_NETWORK_CONFIG_ENABLED", "false")
        .spawn()
        .expect("Failed to spawn sd-agent");

    std::thread::sleep(std::time::Duration::from_millis(500));

    assert!(
        !marker_file.exists(),
        "Env var set to 'false' should NOT trigger fallback"
    );

    child.kill().ok();
    child.wait().expect("Failed to wait on sd-agent");
}

#[test]
fn test_env_var_zero_no_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Create empty config file
    let mut config_file = NamedTempFile::new().unwrap();
    config_file.write_all(b"").unwrap();
    config_file.flush().unwrap();

    // Env var set to "0" should NOT trigger fallback
    let mut child = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .arg(format!("--config={}", config_file.path().display()))
        .env("DD_DISCOVERY_USE_SD_AGENT", "true")
        .env("DD_DISCOVERY_ENABLED", "true")
        .env("DD_NETWORK_CONFIG_ENABLED", "0")
        .spawn()
        .expect("Failed to spawn sd-agent");

    std::thread::sleep(std::time::Duration::from_millis(500));

    assert!(
        !marker_file.exists(),
        "Env var set to '0' should NOT trigger fallback"
    );

    child.kill().ok();
    child.wait().expect("Failed to wait on sd-agent");
}

#[test]
fn test_env_var_non_boolean_triggers_fallback() {
    let temp_dir = TempDir::new().unwrap();
    let marker_file = temp_dir.path().join("sp-called");

    let mock_sp_source = mock_system_probe_path();

    // Env var set to a non-boolean value should trigger fallback (safety net)
    let _output = Command::new(SD_AGENT_BIN)
        .arg("--")
        .arg(&mock_sp_source)
        .arg(&marker_file)
        .arg("run")
        .env("DD_NETWORK_CONFIG_ENABLED", "maybe")
        .output()
        .expect("Failed to execute sd-agent");

    assert!(
        marker_file.exists(),
        "Env var with non-boolean value should trigger fallback as safety net"
    );
}
