// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Fleet processes.d template checks for Windows security-agent (Windows schema defaults).

use crate::config::ProcessConfig;
use crate::config_gate::{condition_config_any_met, gated_key_names, test_env_guard};
use crate::fleet_template_support::{INSTALL_DIR, scm_service_keys, sorted, write_gated_files};
use std::path::Path;

const SECURITY_NAME: &str = "datadog-agent-security";

const SECURITY_TEMPLATE: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-security-windows.yaml.tmpl"
);

const EMPTY_AGENT_YAML: &str = "api_key: 0000001\n";
const EMPTY_SYSPROBE_YAML: &str = "# no modules enabled\n";

#[test]
fn fleet_security_template_declares_legacy_scm_gate() {
    let etc = tempfile::tempdir().expect("tempdir");
    let config = load_template(etc.path());

    let binary = format!("{INSTALL_DIR}/bin/agent/security-agent.exe");
    assert_eq!(config.command, binary);
    assert_eq!(
        config.condition_path_exists.as_deref(),
        Some(binary.as_str())
    );
    assert_eq!(
        config.args,
        vec![
            "start".to_owned(),
            "-c".to_owned(),
            format!("{}/datadog.yaml", etc.path().display()),
            "-c".to_owned(),
            format!("{}/security-agent.yaml", etc.path().display()),
            "--sysprobe-config".to_owned(),
            format!("{}/system-probe.yaml", etc.path().display()),
        ]
    );
    assert!(
        config.auto_start,
        "cutover ships auto_start true together with SCM suppression"
    );

    let (core_keys, sysprobe_keys) = scm_service_keys("cws");
    assert!(
        core_keys.is_empty(),
        "the cws service only reads system-probe.yaml, got core keys {core_keys:?}"
    );
    assert_eq!(
        sysprobe_keys,
        vec!["runtime_security_config.enabled".to_owned()],
        "the cws service has exactly one key"
    );

    let gate = &config.condition_config_any;
    assert_eq!(gate.len(), 1, "expected one gated file, got {gate:?}");
    assert_eq!(
        gate[0].path,
        format!("{}/system-probe.yaml", etc.path().display())
    );
    assert_eq!(
        sorted(&gate[0].keys),
        sorted(&sysprobe_keys),
        "the template must match the cws service keys exactly, got {:?}",
        gate[0].keys
    );

    assert_eq!(config.restart.to_string(), "on-failure");
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
}

#[test]
fn fleet_security_template_names_only_evaluable_keys() {
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

#[test]
fn fleet_security_template_gate_closed_on_default_install() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_gated_files(etc.path(), EMPTY_AGENT_YAML, EMPTY_SYSPROBE_YAML);
    let gate = load_template(etc.path()).condition_config_any;

    assert!(!condition_config_any_met(&gate));
}

#[test]
fn fleet_security_template_gate_opens_when_runtime_security_enabled() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_gated_files(
        etc.path(),
        EMPTY_AGENT_YAML,
        "runtime_security_config:\n  enabled: true\n",
    );
    let gate = load_template(etc.path()).condition_config_any;

    assert!(condition_config_any_met(&gate));
}

fn load_template(etc: &Path) -> ProcessConfig {
    crate::fleet_template_support::load_template(SECURITY_TEMPLATE, SECURITY_NAME, etc)
}
