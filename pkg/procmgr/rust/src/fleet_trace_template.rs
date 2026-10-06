// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Checks on the Windows processes.d entry the fleet installer ships for trace-agent.
//! The template is consumed as the file a human edits and rendered the way the installer
//! renders it. The gate is evaluated against the Windows schema defaults, which is why
//! these tests only run on Windows.

use crate::config::ProcessConfig;
use crate::config_gate::{condition_config_any_met, gated_key_names, test_env_guard};
use crate::fleet_template_support::{INSTALL_DIR, scm_service_keys, sorted};
use std::path::Path;

/// Catalog name of the shipped entry, from `datadog-agent-trace.yaml` in `processes.d`.
const TRACE_NAME: &str = "datadog-agent-trace";

const TRACE_TEMPLATE: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-trace-windows.yaml.tmpl"
);

/// Gate keys all absent, so each one falls through to the Agent's schema default.
const EMPTY_AGENT_YAML: &str = "api_key: 0000001\n";

/// Both gate keys pinned false. Leaving either absent lets its default reopen the gate:
/// `apm_config.enabled` defaults to true.
const DISABLED_AGENT_YAML: &str = concat!(
    "api_key: 0000001\n",
    "apm_config:\n",
    "  enabled: false\n",
    "  error_tracking_standalone:\n    enabled: false\n",
);

#[test]
fn fleet_trace_template_declares_legacy_scm_gate() {
    let etc = tempfile::tempdir().expect("tempdir");
    let config = load_template(etc.path());

    let binary = format!("{INSTALL_DIR}/bin/agent/trace-agent.exe");
    assert_eq!(config.command, binary);
    assert_eq!(
        config.condition_path_exists.as_deref(),
        Some(binary.as_str())
    );
    assert_eq!(
        config.args,
        vec![
            "--config".to_owned(),
            format!("{}/datadog.yaml", etc.path().display())
        ]
    );
    assert!(
        config.auto_start,
        "cutover ships auto_start true together with SCM suppression"
    );

    let (core_keys, sysprobe_keys) = scm_service_keys("apm");
    assert!(
        sysprobe_keys.is_empty(),
        "the apm Servicedef only reads datadog.yaml, got sysprobe keys {sysprobe_keys:?}"
    );

    let gate = &config.condition_config_any;
    assert_eq!(gate.len(), 1, "expected one gated file, got {gate:?}");
    assert_eq!(
        gate[0].path,
        format!("{}/datadog.yaml", etc.path().display())
    );

    assert_eq!(
        sorted(&gate[0].keys),
        sorted(&core_keys),
        "the template must transcribe the apm Servicedef keys, got {:?}",
        gate[0].keys
    );

    assert_eq!(config.restart.to_string(), "on-failure");
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
}

/// An unknown key resolves false and warns on every evaluation, which would leave part of
/// the transcription dead while the template still read like one.
#[test]
fn fleet_trace_template_names_only_evaluable_keys() {
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

/// A default install leaves `apm_config.enabled` true, so the gate opens even though
/// standalone Error Tracking is off.
#[test]
fn fleet_trace_template_gate_opens_on_default_install() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(etc.path(), EMPTY_AGENT_YAML);
    let gate = load_template(etc.path()).condition_config_any;

    assert!(condition_config_any_met(&gate));
}

/// Closing APM and standalone together is what an operator who turned tracing off looks
/// like. Either key left at its default reopens the gate.
#[test]
fn fleet_trace_template_gate_closed_when_apm_disabled() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(etc.path(), DISABLED_AGENT_YAML);
    let gate = load_template(etc.path()).condition_config_any;

    assert!(!condition_config_any_met(&gate));
}

/// Error Tracking standalone is the other half of `utils.IsAPMEnabled`. The template and
/// the SCM fallback both honor it.
#[test]
fn fleet_trace_template_gate_opens_on_error_tracking_standalone() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_agent_yaml(
        etc.path(),
        concat!(
            "apm_config:\n",
            "  enabled: false\n",
            "  error_tracking_standalone:\n    enabled: true\n",
        ),
    );
    let gate = load_template(etc.path()).condition_config_any;

    assert!(condition_config_any_met(&gate));
}

fn write_agent_yaml(etc: &Path, body: &str) {
    std::fs::write(etc.join("datadog.yaml"), body).expect("write datadog.yaml");
}

fn load_template(etc: &Path) -> ProcessConfig {
    crate::fleet_template_support::load_template(TRACE_TEMPLATE, TRACE_NAME, etc)
}
