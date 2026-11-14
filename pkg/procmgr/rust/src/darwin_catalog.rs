// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Loads the generated macOS system-probe `processes.d` entry. It has no host dependency, so the
//! same test runs on every platform.

use crate::config::{RestartPolicy, load_configs};
use std::fs;

const SYSTEM_PROBE: &str = "/opt/datadog-agent/embedded/bin/system-probe";

const SYSPROBE_YAML: &str = include_str!(
    "../../../../pkg/fleet/installer/packages/embedded/tmpl/gen/darwin-pm/processes.d/datadog-agent-sysprobe.yaml"
);

#[test]
fn darwin_sysprobe_entry_is_inert() {
    let dir = tempfile::tempdir().expect("tempdir");
    fs::write(
        dir.path().join("datadog-agent-sysprobe.yaml"),
        SYSPROBE_YAML,
    )
    .expect("write datadog-agent-sysprobe.yaml");
    let catalog = load_configs(dir.path()).expect("darwin catalog parses");
    assert!(
        catalog.invalid.is_empty(),
        "invalid darwin configs: {:?}",
        catalog
            .invalid
            .iter()
            .map(|entry| format!("{}: {}", entry.name, entry.error))
            .collect::<Vec<_>>()
    );
    assert_eq!(catalog.processes.len(), 1);
    let process = &catalog.processes[0];
    assert_eq!(process.name, "datadog-agent-sysprobe");
    let config = &process.config;

    assert!(!config.auto_start);

    assert_eq!(config.command, SYSTEM_PROBE);
    assert_eq!(config.condition_path_exists.as_deref(), Some(SYSTEM_PROBE));
    assert_eq!(config.args, ["run", "-c", "/opt/datadog-agent/etc"]);

    assert_eq!(config.restart, RestartPolicy::OnFailure);
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));

    assert!(
        config.condition_config_any.is_empty(),
        "condition_config_any: {:?}",
        config.condition_config_any
    );
    assert!(
        config.condition_config_none.is_empty(),
        "condition_config_none: {:?}",
        config.condition_config_none
    );

    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
    assert!(config.env.is_empty(), "env: {:?}", config.env);
}
