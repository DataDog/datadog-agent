// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Checks on the Windows processes.d entry the fleet installer ships for system-probe.
//! The template is consumed as the file a human edits and rendered the way the installer
//! renders it. The gate is evaluated against the Windows schema defaults, which is why
//! these tests only run on Windows.

use crate::config::ProcessConfig;
use crate::config_gate::{condition_config_any_met, gated_key_names, test_env_guard};
use crate::fleet_template_support::{INSTALL_DIR, scm_service_keys, sorted, write_gated_files};
use std::path::Path;

/// Catalog name of the shipped entry, from `datadog-agent-sysprobe.yaml` in `processes.d`.
const SYSPROBE_NAME: &str = "datadog-agent-sysprobe";

const SYSPROBE_TEMPLATE: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-sysprobe-windows.yaml.tmpl"
);

/// Gate keys all absent, so each one falls through to the Agent's schema default.
const EMPTY_AGENT_YAML: &str = "api_key: 0000001\n";
const EMPTY_SYSPROBE_YAML: &str = "# no modules enabled\n";

#[test]
fn fleet_sysprobe_template_declares_legacy_scm_gate() {
    let etc = tempfile::tempdir().expect("tempdir");
    let config = load_template(etc.path());

    let binary = format!("{INSTALL_DIR}/bin/agent/system-probe.exe");
    assert_eq!(config.command, binary);
    assert_eq!(
        config.condition_path_exists.as_deref(),
        Some(binary.as_str())
    );
    // system-probe prepends `run` itself when its parent is not services.exe, and the MSI
    // registers datadog-system-probe with no arguments either.
    assert!(
        config.args.is_empty(),
        "expected no args, got {:?}",
        config.args
    );
    assert!(
        !config.auto_start,
        "the core Agent still starts datadog-system-probe through the SCM, so auto-starting \
         here would run two system-probes"
    );

    let (core_keys, sysprobe_keys) = scm_service_keys("sysprobe");
    let gate = &config.condition_config_any;
    assert_eq!(
        gate.len(),
        2,
        "the gate must split across system-probe.yaml and datadog.yaml, got {gate:?}"
    );
    assert_eq!(
        gate[0].path,
        format!("{}/system-probe.yaml", etc.path().display())
    );
    assert_eq!(
        sorted(&gate[0].keys),
        sysprobe_keys,
        "drift from the SCM sysprobeConf keys"
    );
    assert_eq!(
        gate[1].path,
        format!("{}/datadog.yaml", etc.path().display())
    );
    assert_eq!(
        sorted(&gate[1].keys),
        core_keys,
        "drift from the SCM coreConf keys"
    );

    // on-failure is load-bearing: a system-probe that finds itself disabled exits 0, and
    // `always` would turn that into a permanent respawn loop.
    assert_eq!(config.restart.to_string(), "on-failure");
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));
    // The Windows privileged spawn profile rejects anything but inherit or null.
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
}

/// An unknown key resolves false and warns on every evaluation, which would leave part of
/// the transcription dead while the template still read like one.
#[test]
fn fleet_sysprobe_template_names_only_evaluable_keys() {
    let etc = tempfile::tempdir().expect("tempdir");
    let evaluable = gated_key_names();
    for file in &load_template(etc.path()).condition_config_any {
        for key in &file.keys {
            assert!(
                evaluable.contains(&key.as_str()),
                "{key} is not in the gate's key table, so it would resolve false and warn \
                 on every evaluation; add a GatedKeySpec for it"
            );
        }
    }
}

/// A default install enables no system-probe module on Windows, so the gate stays closed,
/// as the SCM leaves datadog-system-probe stopped.
#[test]
fn fleet_sysprobe_template_gate_closed_on_default_install() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_gated_files(etc.path(), EMPTY_AGENT_YAML, EMPTY_SYSPROBE_YAML);

    assert!(!condition_config_any_met(
        &load_template(etc.path()).condition_config_any
    ));
}

/// Each of the five SCM keys opens the gate on its own, from the file the SCM reads it
/// from.
#[test]
fn fleet_sysprobe_template_gate_opens_on_each_scm_key() {
    for (agent_yaml, sysprobe_yaml) in [
        (EMPTY_AGENT_YAML, "network_config:\n  enabled: true\n"),
        (EMPTY_AGENT_YAML, "system_probe_config:\n  enabled: true\n"),
        (
            EMPTY_AGENT_YAML,
            "windows_crash_detection:\n  enabled: true\n",
        ),
        (
            EMPTY_AGENT_YAML,
            "runtime_security_config:\n  enabled: true\n",
        ),
        (
            "software_inventory:\n  enabled: true\n",
            EMPTY_SYSPROBE_YAML,
        ),
    ] {
        let _env = test_env_guard();
        let etc = tempfile::tempdir().expect("tempdir");
        write_gated_files(etc.path(), agent_yaml, sysprobe_yaml);

        assert!(
            condition_config_any_met(&load_template(etc.path()).condition_config_any),
            "gate stayed closed for datadog.yaml={agent_yaml:?} system-probe.yaml={sysprobe_yaml:?}"
        );
    }
}

fn load_template(etc: &Path) -> ProcessConfig {
    crate::fleet_template_support::load_template(SYSPROBE_TEMPLATE, SYSPROBE_NAME, etc)
}
