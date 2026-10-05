// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Checks on the Windows processes.d entry the fleet installer ships for Agent Data Plane.
//! The template is consumed as the file a human edits and rendered the way the installer
//! renders it. The gate is evaluated against the Windows schema defaults, which is why
//! these tests only run on Windows.

use crate::config::ProcessConfig;
use crate::config_gate::{condition_config_any_met, gated_key_names, test_env_guard};
use crate::fleet_template_support::{INSTALL_DIR, sorted};
use std::path::Path;

const DATA_PLANE_TEMPLATE: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-data-plane-windows.yaml.tmpl"
);
const DATA_PLANE_NAME: &str = "datadog-agent-data-plane";

#[test]
fn fleet_data_plane_template_declares_adp_startup_gate() {
    let etc = tempfile::tempdir().expect("tempdir");
    let config = load_template(etc.path());

    let binary = format!("{INSTALL_DIR}/bin/agent/agent-data-plane.exe");
    assert_eq!(config.command, binary);
    assert_eq!(
        config.condition_path_exists.as_deref(),
        Some(binary.as_str())
    );
    assert!(config.auto_start);

    let gate = &config.condition_config_any;
    assert_eq!(gate.len(), 1, "expected one gated file, got {gate:?}");
    assert_eq!(
        gate[0].path,
        format!("{}/datadog.yaml", etc.path().display())
    );
    assert_eq!(sorted(&gate[0].keys), ["data_plane.enabled"]);
    assert!(config.condition_config_none.is_empty());

    // ADP exits 0 when it is not enabled, and `always` would respawn that forever.
    assert_eq!(config.restart.to_string(), "on-failure");
}

/// An unknown key resolves false, so a misspelled one would leave ADP unable to start
/// even when enabled.
#[test]
fn fleet_data_plane_template_names_only_evaluable_keys() {
    let etc = tempfile::tempdir().expect("tempdir");
    let evaluable = gated_key_names();
    for file in &load_template(etc.path()).condition_config_any {
        for key in &file.keys {
            assert!(
                evaluable.contains(&key.as_str()),
                "{key} is not a gated key, so it can never open the gate"
            );
        }
    }
}

#[test]
fn fleet_data_plane_template_gate_closed_on_default_install() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(etc.path(), "api_key: 0000001\n");
    let gate = load_template(etc.path()).condition_config_any;

    assert!(
        !condition_config_any_met(&gate),
        "data_plane.enabled defaults to false, so ADP must not be spawned"
    );
}

#[test]
fn fleet_data_plane_template_gate_opens_when_enabled() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(etc.path(), "data_plane:\n  enabled: true\n");
    let gate = load_template(etc.path()).condition_config_any;

    assert!(condition_config_any_met(&gate));
}

/// Standalone mode is not a customer path under procmgr, so it must not open the gate.
#[test]
fn fleet_data_plane_template_gate_ignores_standalone_mode() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(etc.path(), "data_plane:\n  standalone_mode: true\n");
    let gate = load_template(etc.path()).condition_config_any;

    assert!(
        !condition_config_any_met(&gate),
        "standalone_mode alone must not start ADP under procmgr"
    );
}

fn write_agent_yaml(etc: &Path, body: &str) {
    std::fs::write(etc.join("datadog.yaml"), body).expect("write datadog.yaml");
}

fn load_template(etc: &Path) -> ProcessConfig {
    crate::fleet_template_support::load_template(DATA_PLANE_TEMPLATE, DATA_PLANE_NAME, etc)
}
