// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Parse tests for the all-in-one `processes.d/aio` catalog. The checkout test is skipped under Bazel.

use crate::config::{ProcessConfig, RestartPolicy, load_configs};
use std::collections::HashMap;
use std::fs;
#[cfg(not(bazel))]
use std::path::Path;

const PATH_ENV: &str = "/opt/datadog-agent/bin/agent:/opt/datadog-agent/embedded/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin";

const AGENT_BIN: &str = "/opt/datadog-agent/bin/agent/agent";
const TRACE_LOADER: &str = "/opt/datadog-agent/embedded/bin/trace-loader";
const TRACE_AGENT: &str = "/opt/datadog-agent/embedded/bin/trace-agent";
const PROCESS_AGENT: &str = "/opt/datadog-agent/embedded/bin/process-agent";
const SYSTEM_PROBE: &str = "/opt/datadog-agent/embedded/bin/system-probe";
const SECURITY_AGENT: &str = "/opt/datadog-agent/embedded/bin/security-agent";
const OTEL_AGENT: &str = "/opt/datadog-agent/embedded/bin/otel-agent";
const DATA_PLANE: &str = "/opt/datadog-agent/embedded/bin/agent-data-plane";

const CATALOG: &[(&str, &str)] = &[
    (
        "datadog-agent.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent.yaml"),
    ),
    (
        "datadog-agent-trace.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-trace.yaml"),
    ),
    (
        "datadog-agent-process.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-process.yaml"),
    ),
    (
        "datadog-agent-sysprobe.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-sysprobe.yaml"),
    ),
    (
        "datadog-agent-security.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-security.yaml"),
    ),
    (
        "datadog-agent-otel.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-otel.yaml"),
    ),
    (
        "datadog-agent-data-plane.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-data-plane.yaml"),
    ),
];

const AUXILIARIES: &[&str] = &[
    "datadog-agent-trace",
    "datadog-agent-process",
    "datadog-agent-sysprobe",
    "datadog-agent-security",
    "datadog-agent-otel",
    "datadog-agent-data-plane",
];

fn load_shipped_catalog() -> HashMap<String, ProcessConfig> {
    let dir = tempfile::tempdir().expect("tempdir");
    for (name, body) in CATALOG {
        fs::write(dir.path().join(name), body).unwrap_or_else(|err| {
            panic!("write {name}: {err}");
        });
    }
    let catalog = load_configs(dir.path()).expect("aio catalog parses");
    assert!(
        catalog.invalid.is_empty(),
        "invalid aio configs: {:?}",
        catalog
            .invalid
            .iter()
            .map(|entry| format!("{}: {}", entry.name, entry.error))
            .collect::<Vec<_>>()
    );
    catalog
        .processes
        .into_iter()
        .map(|process| (process.name, process.config))
        .collect()
}

fn assert_shared_stdio(config: &ProcessConfig) {
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
    assert_eq!(config.env.get("PATH").map(String::as_str), Some(PATH_ENV));
    assert!(config.after.is_empty(), "after: {:?}", config.after);
    assert!(config.before.is_empty(), "before: {:?}", config.before);
    assert_eq!(config.stop_timeout, None);
    assert!(config.auto_start);
}

#[test]
fn aio_catalog_parses_with_load_configs() {
    let configs = load_shipped_catalog();
    assert_eq!(configs.len(), CATALOG.len());
}

#[test]
fn aio_core_agent_is_the_critical_process() {
    let configs = load_shipped_catalog();
    let core = configs.get("datadog-agent").expect("datadog-agent");

    assert_eq!(core.restart, RestartPolicy::Never);
    assert!(core.exit_with_supervisor);
    assert_eq!(core.command, AGENT_BIN);
    assert_eq!(core.args, ["run"]);
    assert_eq!(core.condition_path_exists, None);
    assert!(core.condition_config_any.is_empty());
    assert!(core.condition_config_none.is_empty());
    assert_eq!(core.restart_sec, None);
    assert_shared_stdio(core);

    let critical: Vec<_> = configs
        .iter()
        .filter(|(_, config)| config.exit_with_supervisor)
        .map(|(name, _)| name.as_str())
        .collect();
    assert_eq!(critical, ["datadog-agent"]);
}

#[test]
fn aio_auxiliaries_restart_on_failure() {
    let configs = load_shipped_catalog();

    for name in AUXILIARIES {
        let config = configs
            .get(*name)
            .unwrap_or_else(|| panic!("missing {name}"));
        assert_eq!(config.restart, RestartPolicy::OnFailure, "{name} restart");
        assert_eq!(config.restart_sec, Some(2.0), "{name} restart_sec");
        assert_eq!(
            config.start_limit_interval_sec,
            Some(10),
            "{name} start_limit_interval_sec"
        );
        assert_eq!(
            config.start_limit_burst,
            Some(5),
            "{name} start_limit_burst"
        );
        assert!(
            !config.exit_with_supervisor,
            "{name} must not exit with the supervisor"
        );
        assert!(
            config.command.starts_with('/'),
            "{name} command is not absolute: {}",
            config.command
        );
        assert_shared_stdio(config);
    }
}

#[test]
fn aio_optional_binaries_require_condition_path_exists() {
    let configs = load_shipped_catalog();
    for (name, path) in [
        ("datadog-agent-otel", OTEL_AGENT),
        ("datadog-agent-data-plane", DATA_PLANE),
        ("datadog-agent-sysprobe", SYSTEM_PROBE),
    ] {
        let config = configs
            .get(name)
            .unwrap_or_else(|| panic!("missing {name}"));
        assert_eq!(
            config.condition_path_exists.as_deref(),
            Some(path),
            "{name} condition_path_exists"
        );
    }
}

#[test]
fn aio_commands_match_s6_run_scripts() {
    let configs = load_shipped_catalog();

    let trace = configs.get("datadog-agent-trace").unwrap();
    assert_eq!(trace.command, TRACE_LOADER);
    assert_eq!(
        trace.args,
        [
            "/etc/datadog-agent/datadog.yaml",
            TRACE_AGENT,
            "--config=/etc/datadog-agent/datadog.yaml",
        ]
    );
    assert_eq!(trace.condition_path_exists.as_deref(), Some(TRACE_AGENT));

    let process = configs.get("datadog-agent-process").unwrap();
    assert_eq!(process.command, PROCESS_AGENT);
    assert_eq!(process.args, ["--cfgpath=/etc/datadog-agent/datadog.yaml"]);
    assert_eq!(
        process.condition_path_exists.as_deref(),
        Some(PROCESS_AGENT)
    );

    let sysprobe = configs.get("datadog-agent-sysprobe").unwrap();
    assert_eq!(sysprobe.command, SYSTEM_PROBE);
    assert_eq!(
        sysprobe.args,
        ["run", "--config=/etc/datadog-agent/system-probe.yaml"]
    );
    let sysprobe_keys: Vec<&str> = sysprobe
        .condition_config_any
        .iter()
        .flat_map(|file| file.keys.iter().map(String::as_str))
        .collect();
    assert!(
        !sysprobe_keys.contains(&"windows_crash_detection.enabled"),
        "windows_crash_detection.enabled is Windows-only: {sysprobe_keys:?}"
    );
    assert_eq!(
        sysprobe
            .condition_config_none
            .iter()
            .flat_map(|file| file.keys.iter().map(String::as_str))
            .collect::<Vec<_>>(),
        ["system_probe_config.external"]
    );

    let security = configs.get("datadog-agent-security").unwrap();
    assert_eq!(security.command, SECURITY_AGENT);
    assert_eq!(
        security.args,
        [
            "start",
            "-c=/etc/datadog-agent/datadog.yaml",
            "--sysprobe-config=/etc/datadog-agent/system-probe.yaml",
        ]
    );
    assert_eq!(
        security.condition_path_exists.as_deref(),
        Some(SECURITY_AGENT)
    );
    assert_eq!(
        security
            .condition_config_any
            .iter()
            .flat_map(|file| file.keys.iter().map(String::as_str))
            .collect::<Vec<_>>(),
        [
            "compliance_config.enabled",
            "runtime_security_config.enabled",
        ]
    );

    let otel = configs.get("datadog-agent-otel").unwrap();
    assert_eq!(otel.command, OTEL_AGENT);
    assert_eq!(
        otel.args,
        ["--config", "/etc/datadog-agent/otel-config.yaml"]
    );

    let data_plane = configs.get("datadog-agent-data-plane").unwrap();
    assert_eq!(data_plane.command, DATA_PLANE);
    assert_eq!(
        data_plane.args,
        ["--config", "/etc/datadog-agent/datadog.yaml", "run"]
    );
    assert!(
        !data_plane.args.iter().any(|arg| arg == "--pidfile"),
        "s6 does not pass --pidfile: {:?}",
        data_plane.args
    );
}

#[cfg(not(bazel))]
#[test]
fn aio_directory_keeps_par_beside_the_new_catalog() {
    let processes_d =
        Path::new(env!("CARGO_MANIFEST_DIR")).join("../../../Dockerfiles/agent/processes.d");
    let aio = processes_d.join("aio");

    for name in [
        "datadog-agent-par-control.yaml",
        "datadog-agent-action-executor.yaml",
    ] {
        let link = processes_d.join(name);
        let target = fs::read_link(&link).unwrap_or_else(|err| panic!("read_link {name}: {err}"));
        assert_eq!(target, Path::new("aio").join(name), "{name} symlink target");
        assert!(
            fs::symlink_metadata(aio.join(name))
                .unwrap_or_else(|err| panic!("stat aio {name}: {err}"))
                .is_file(),
            "{name} in aio should be the regular file"
        );
    }

    let top = load_configs(&processes_d).expect("top-level processes.d parses");
    assert!(top.invalid.is_empty());
    let mut top_names: Vec<_> = top
        .processes
        .iter()
        .map(|process| process.name.as_str())
        .collect();
    top_names.sort_unstable();
    assert_eq!(
        top_names,
        ["datadog-agent-action-executor", "datadog-agent-par-control",]
    );

    let catalog = load_configs(&aio).expect("aio directory parses");
    assert!(catalog.invalid.is_empty());
    let mut names: Vec<_> = catalog
        .processes
        .iter()
        .map(|process| process.name.as_str())
        .collect();
    names.sort_unstable();
    assert_eq!(
        names,
        [
            "datadog-agent",
            "datadog-agent-action-executor",
            "datadog-agent-data-plane",
            "datadog-agent-otel",
            "datadog-agent-par-control",
            "datadog-agent-process",
            "datadog-agent-security",
            "datadog-agent-sysprobe",
            "datadog-agent-trace",
        ]
    );

    let mut top_level = Vec::new();
    for entry in fs::read_dir(&processes_d).expect("read processes.d") {
        let entry = entry.expect("processes.d entry");
        let path = entry.path();
        if path.extension().is_some_and(|ext| ext == "yaml") {
            top_level.push(entry.file_name().to_string_lossy().into_owned());
        }
    }
    top_level.sort();
    assert_eq!(
        top_level,
        [
            "datadog-agent-action-executor.yaml".to_string(),
            "datadog-agent-par-control.yaml".to_string(),
        ]
    );
}
