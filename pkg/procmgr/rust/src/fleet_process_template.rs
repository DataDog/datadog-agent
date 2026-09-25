// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Checks on the Windows processes.d entry the fleet installer ships for process-agent.
//! The template is consumed as the file a human edits and rendered the way the installer
//! renders it. The gate is evaluated against the Windows schema defaults, which is why
//! these tests only run on Windows.

use crate::config::{ProcessConfig, load_configs};
use crate::config_gate::{condition_config_any_met, test_env_guard};
use crate::spawn::DATADOG_AGENT_PROCESS;
use std::path::Path;

const PROCESS_TEMPLATE: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-process-windows.yaml.tmpl"
);

/// The legacy SCM service definition the template transcribes. Parsed rather than
/// duplicated so that editing the SCM key list without editing the template fails here.
const SCM_SERVICES_GO: &str =
    include_str!("../../../../cmd/agent/subcommands/run/dependent_services_windows.go");

const INSTALL_DIR: &str = "C:/Program Files/Datadog/Datadog Agent";

/// Gate keys all absent, so each one falls through to the Agent's schema default.
const EMPTY_AGENT_YAML: &str = "api_key: 0000001\n";
const EMPTY_SYSPROBE_YAML: &str = "# no modules enabled\n";

/// Every gated key pinned false, which is what an operator who turned process collection
/// off looks like. Each one has to be written out, since leaving one absent lets its
/// default reopen the gate on its own.
const DISABLED_AGENT_YAML: &str = concat!(
    "api_key: 0000001\n",
    "process_config:\n",
    "  enabled: false\n",
    "  process_collection:\n    enabled: false\n",
    "  container_collection:\n    enabled: false\n",
    "  process_discovery:\n    enabled: false\n",
);
const DISABLED_SYSPROBE_YAML: &str = concat!(
    "network_config:\n  enabled: false\n",
    "system_probe_config:\n  enabled: false\n",
);

#[test]
fn fleet_process_template_declares_legacy_scm_gate() {
    let etc = tempfile::tempdir().expect("tempdir");
    let config = load_template(etc.path());

    let binary = format!("{INSTALL_DIR}/bin/agent/process-agent.exe");
    assert_eq!(config.command, binary);
    assert_eq!(
        config.condition_path_exists.as_deref(),
        Some(binary.as_str())
    );
    assert!(
        config.auto_start,
        "the Agent suppresses the legacy SCM service whenever this entry is installed"
    );

    let (core_keys, sysprobe_keys) = scm_process_keys();
    let gate = &config.condition_config_any;
    assert_eq!(
        gate.len(),
        2,
        "the gate must split across datadog.yaml and system-probe.yaml, got {gate:?}"
    );
    assert_eq!(
        gate[0].path,
        format!("{}/datadog.yaml", etc.path().display())
    );
    assert_eq!(
        sorted(&gate[0].keys),
        core_keys,
        "drift from the SCM coreConf keys"
    );
    assert_eq!(
        gate[1].path,
        format!("{}/system-probe.yaml", etc.path().display())
    );
    assert_eq!(
        sorted(&gate[1].keys),
        sysprobe_keys,
        "drift from the SCM sysprobeConf keys"
    );

    assert_eq!(config.restart.to_string(), "on-failure");
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));
    // The Windows privileged spawn profile rejects anything but inherit or null.
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
}

/// A default install leaves the gate open, which is what makes suppressing the legacy SCM
/// service safe. The core half alone has to open it: system-probe has no module enabled by
/// default on Windows.
#[test]
fn fleet_process_template_gate_opens_on_default_install() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_gated_files(etc.path(), EMPTY_AGENT_YAML, EMPTY_SYSPROBE_YAML);
    let gate = load_template(etc.path()).condition_config_any;

    assert!(
        condition_config_any_met(&gate[..1]),
        "the core process_config defaults must open the gate on their own"
    );
    assert!(
        !condition_config_any_met(&gate[1..]),
        "no system-probe module is enabled by default on Windows"
    );
    assert!(condition_config_any_met(&gate));
}

/// Now that the Agent suppresses the legacy SCM service whenever this entry is installed,
/// procmgr is the only thing left that could run process-agent, so an operator turning
/// process collection off has to keep it from starting here.
#[test]
fn fleet_process_template_gate_closed_when_collection_disabled() {
    let _env = test_env_guard();
    let etc = tempfile::tempdir().expect("tempdir");
    write_gated_files(etc.path(), DISABLED_AGENT_YAML, DISABLED_SYSPROBE_YAML);
    let gate = load_template(etc.path()).condition_config_any;

    assert!(!condition_config_any_met(&gate));
}

fn write_gated_files(etc: &Path, agent_yaml: &str, sysprobe_yaml: &str) {
    std::fs::write(etc.join("datadog.yaml"), agent_yaml).expect("write datadog.yaml");
    std::fs::write(etc.join("system-probe.yaml"), sysprobe_yaml).expect("write system-probe.yaml");
}

/// Render the shipped `.tmpl` the way `renderConfig` does at install time, with `etc` as
/// the data root, and load it through the same catalog loader the daemon uses.
fn load_template(etc: &Path) -> ProcessConfig {
    let rendered = PROCESS_TEMPLATE
        .replace("{{.InstallDir}}", INSTALL_DIR)
        .replace("{{.EtcDir}}", &etc.display().to_string());
    assert!(
        !rendered.contains("{{"),
        "the installer only substitutes InstallDir and EtcDir, so the template must use no \
         other placeholder:\n{rendered}"
    );

    let catalog = tempfile::tempdir().expect("tempdir");
    std::fs::write(
        catalog.path().join(format!("{DATADOG_AGENT_PROCESS}.yaml")),
        rendered,
    )
    .expect("write yaml");
    let mut definitions = load_configs(catalog.path()).expect("load catalog");
    assert_eq!(definitions.len(), 1, "expected exactly one process");
    definitions.remove(0).config
}

fn sorted(keys: &[String]) -> Vec<String> {
    let mut sorted = keys.to_vec();
    sorted.sort();
    sorted
}

/// The `configKeys` of the legacy SCM `process` service, sorted and split by the config
/// file each key is read from: `(coreConf keys, sysprobeConf keys)`. A Go map literal has
/// no meaningful order, so only the sets are compared.
fn scm_process_keys() -> (Vec<String>, Vec<String>) {
    let definition = SCM_SERVICES_GO
        .find("name: \"process\",")
        .map(|start| &SCM_SERVICES_GO[start..])
        .expect("no `name: \"process\"` service definition in dependent_services_windows.go");
    let map = definition
        .find("configKeys: map[string]model.Reader{")
        .map(|start| &definition[start..])
        .expect("the SCM process service has no configKeys map");
    let body = &map[..map.find("},").expect("unterminated configKeys map")];

    let mut core = Vec::new();
    let mut sysprobe = Vec::new();
    for line in body.lines().skip(1) {
        let Some(key) = line.split('"').nth(1) else {
            continue;
        };
        if line.contains("sysprobeConf") {
            sysprobe.push(key.to_owned());
        } else if line.contains("coreConf") {
            core.push(key.to_owned());
        } else {
            panic!(
                "cannot tell which config file `{}` is read from",
                line.trim()
            );
        }
    }
    assert!(
        !core.is_empty() && !sysprobe.is_empty(),
        "parsed an implausible SCM key split: core={core:?} sysprobe={sysprobe:?}"
    );
    (sorted(&core), sorted(&sysprobe))
}
