// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Parse tests for the all-in-one `processes.d/aio` catalog.
//!
//! Bazel does not keep symlink identity or git file mode, so the test does not check them.

use crate::config::{ProcessConfig, RestartPolicy, load_configs};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};

const PATH_ENV: &str = "/opt/datadog-agent/bin/agent:/opt/datadog-agent/embedded/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin";

const AGENT_BIN: &str = "/opt/datadog-agent/bin/agent/agent";
const TRACE_LOADER: &str = "/opt/datadog-agent/embedded/bin/trace-loader";
const TRACE_AGENT: &str = "/opt/datadog-agent/embedded/bin/trace-agent";
const PROCESS_AGENT: &str = "/opt/datadog-agent/embedded/bin/process-agent";
const SYSTEM_PROBE: &str = "/opt/datadog-agent/embedded/bin/system-probe";
const SECURITY_AGENT: &str = "/opt/datadog-agent/embedded/bin/security-agent";
const OTEL_AGENT: &str = "/opt/datadog-agent/embedded/bin/otel-agent";
const DATA_PLANE: &str = "/opt/datadog-agent/embedded/bin/agent-data-plane";
const PAR_CONTROL: &str = "/opt/datadog-agent/embedded/bin/par-control";
const PRIVATE_ACTION_RUNNER: &str = "/opt/datadog-agent/embedded/bin/privateactionrunner";

const ENTRYPOINT_DEFAULT: &str =
    include_str!("../../../../Dockerfiles/agent/entrypoint.d/_default");
const ENTRYPOINT_PRIVATE_ACTION_RUNNER: &str =
    include_str!("../../../../Dockerfiles/agent/entrypoint.d/privateactionrunner");
const ENTRYPOINT_PROCMGR_ALL_IN_ONE: &str =
    include_str!("../../../../Dockerfiles/agent/entrypoint.d/procmgr-all-in-one");
const ENTRYPOINT_S6_ALL_IN_ONE: &str =
    include_str!("../../../../Dockerfiles/agent/entrypoint.d/s6-all-in-one");
const AGENT_DOCKERFILE: &str = include_str!("../../../../Dockerfiles/agent/Dockerfile");
const ENTRYPOINT_LAUNCHER: &str = include_str!("../../../../Dockerfiles/agent/entrypoint.sh");

const TOP_LEVEL_YAMLS: &[&str] = &[
    "datadog-agent-action-executor.yaml",
    "datadog-agent-par-control.yaml",
];

const AIO_YAMLS: &[&str] = &[
    "datadog-agent-action-executor.yaml",
    "datadog-agent-data-plane.yaml",
    "datadog-agent-otel.yaml",
    "datadog-agent-par-control.yaml",
    "datadog-agent-par.yaml",
    "datadog-agent-process.yaml",
    "datadog-agent-security.yaml",
    "datadog-agent-sysprobe.yaml",
    "datadog-agent-trace.yaml",
    "datadog-agent.yaml",
];

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
    (
        "datadog-agent-par-control.yaml",
        include_str!(
            "../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-par-control.yaml"
        ),
    ),
    (
        "datadog-agent-par.yaml",
        include_str!("../../../../Dockerfiles/agent/processes.d/aio/datadog-agent-par.yaml"),
    ),
];

const AUXILIARIES: &[&str] = &[
    "datadog-agent-trace",
    "datadog-agent-process",
    "datadog-agent-sysprobe",
    "datadog-agent-security",
    "datadog-agent-otel",
    "datadog-agent-data-plane",
    "datadog-agent-par",
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
fn aio_par_control_skips_when_the_binary_is_absent() {
    let configs = load_shipped_catalog();
    let par_control = configs
        .get("datadog-agent-par-control")
        .expect("par-control");
    assert_eq!(
        par_control.condition_path_exists.as_deref(),
        Some(PAR_CONTROL)
    );
    assert_eq!(par_control.stop_timeout, Some(180));
    assert_eq!(
        par_control
            .condition_config_any
            .iter()
            .flat_map(|file| file.keys.iter().map(String::as_str))
            .collect::<Vec<_>>(),
        ["private_action_runner.split_enabled"]
    );
    assert_eq!(
        par_control
            .condition_config_none
            .iter()
            .flat_map(|file| file.keys.iter().map(String::as_str))
            .collect::<Vec<_>>(),
        ["fips.enabled"]
    );
    assert_eq!(
        par_control
            .env
            .get("DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED")
            .map(String::as_str),
        Some("true")
    );
}

#[test]
fn aio_private_action_runner_monolith_is_gated_on_enabled() {
    let configs = load_shipped_catalog();
    let monolith = configs.get("datadog-agent-par").expect("par monolith");

    assert_eq!(monolith.command, PRIVATE_ACTION_RUNNER);
    assert_eq!(
        monolith.args,
        ["run", "--cfgpath=/etc/datadog-agent/datadog.yaml"]
    );
    assert_eq!(
        monolith.condition_path_exists.as_deref(),
        Some(PRIVATE_ACTION_RUNNER)
    );
    assert_eq!(
        monolith
            .condition_config_any
            .iter()
            .flat_map(|file| file.keys.iter().map(String::as_str))
            .collect::<Vec<_>>(),
        ["private_action_runner.enabled"]
    );
    assert!(
        monolith.condition_config_none.is_empty(),
        "the monolith starts whenever PAR is enabled and stands down itself in split mode"
    );
    assert!(!monolith.exit_with_supervisor);
    let enabled_gated = monolith.condition_config_any.iter().any(|file| {
        file.keys
            .iter()
            .any(|key| key == "private_action_runner.enabled")
    });
    assert!(
        monolith.auto_start && enabled_gated,
        "monolith is not auto_start unless private_action_runner.enabled is present"
    );
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

fn catalog_yaml_files() -> (Vec<PathBuf>, Vec<PathBuf>) {
    #[cfg(bazel)]
    {
        fn resolve(var: &str) -> Vec<PathBuf> {
            let raw = std::env::var(var).unwrap_or_else(|err| panic!("{var} must be set: {err}"));
            let runfiles = runfiles::Runfiles::create()
                .expect("catalog YAML is a runfile, so this test needs `bazel test`");
            let repo = option_env!("REPOSITORY_NAME").unwrap_or("");
            raw.split_whitespace()
                .map(|marker| {
                    runfiles
                        .rlocation_from(marker, repo)
                        .unwrap_or_else(|| panic!("no runfile for {marker}"))
                })
                .collect()
        }
        (resolve("DD_AIO_TOP_LEVEL_YAMLS"), resolve("DD_AIO_YAMLS"))
    }
    #[cfg(not(bazel))]
    {
        // rules_rust rejects CARGO_MANIFEST_DIR under --cfg=bazel, so it stays here.
        let processes_d = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../Dockerfiles/agent/processes.d");
        fn yaml_files(dir: &PathBuf) -> Vec<PathBuf> {
            let mut files = Vec::new();
            for entry in
                fs::read_dir(dir).unwrap_or_else(|err| panic!("read {}: {err}", dir.display()))
            {
                let entry = entry.expect("dir entry");
                let path = entry.path();
                if path.extension().is_some_and(|ext| ext == "yaml") {
                    files.push(path);
                }
            }
            files
        }
        (
            yaml_files(&processes_d),
            yaml_files(&processes_d.join("aio")),
        )
    }
}

fn sorted_file_names(files: &[PathBuf]) -> Vec<String> {
    let mut names: Vec<_> = files
        .iter()
        .map(|path| {
            path.file_name()
                .unwrap_or_else(|| panic!("{} has no file name", path.display()))
                .to_string_lossy()
                .into_owned()
        })
        .collect();
    names.sort();
    names
}

fn file_name(path: &Path) -> String {
    path.file_name()
        .unwrap_or_else(|| panic!("{} has no file name", path.display()))
        .to_string_lossy()
        .into_owned()
}

fn read_by_name(files: &[PathBuf]) -> HashMap<String, Vec<u8>> {
    files
        .iter()
        .map(|path| {
            let bytes =
                fs::read(path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()));
            (file_name(path), bytes)
        })
        .collect()
}

/// Windows checks a symlink out as a file holding its target path, `aio/<name>`.
fn symlink_checkout_bytes(
    bytes: &[u8],
    name: &str,
    aio_bytes: &HashMap<String, Vec<u8>>,
) -> Option<Vec<u8>> {
    let text = std::str::from_utf8(bytes).ok()?.trim();
    if text.contains('\n') || text.contains(':') {
        return None;
    }
    let forward = format!("aio/{name}");
    let backward = format!("aio\\{name}");
    if text != forward && text != backward {
        return None;
    }
    aio_bytes.get(name).cloned()
}

fn read_top_level(
    files: &[PathBuf],
    aio_bytes: &HashMap<String, Vec<u8>>,
) -> HashMap<String, Vec<u8>> {
    files
        .iter()
        .map(|path| {
            let name = file_name(path);
            let bytes =
                fs::read(path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()));
            let bytes = symlink_checkout_bytes(&bytes, &name, aio_bytes).unwrap_or(bytes);
            (name, bytes)
        })
        .collect()
}

#[test]
fn symlink_checkout_text_resolves_to_the_aio_file() {
    let name = "datadog-agent-action-executor.yaml";
    let aio = HashMap::from([(name.to_string(), b"description: executor\n".to_vec())]);
    let link = format!("aio/{name}");
    assert_eq!(
        symlink_checkout_bytes(link.as_bytes(), name, &aio).as_deref(),
        Some(b"description: executor\n".as_slice())
    );
    let windows = format!("aio\\{name}");
    assert_eq!(
        symlink_checkout_bytes(windows.as_bytes(), name, &aio).as_deref(),
        Some(b"description: executor\n".as_slice())
    );
    assert!(symlink_checkout_bytes(b"description: executor\n", name, &aio).is_none());
    assert!(symlink_checkout_bytes(b"aio/other.yaml", name, &aio).is_none());
}

fn stage_named_bytes(files: &HashMap<String, Vec<u8>>) -> tempfile::TempDir {
    let dir = tempfile::tempdir().expect("tempdir");
    for (name, bytes) in files {
        fs::write(dir.path().join(name), bytes).unwrap_or_else(|err| panic!("write {name}: {err}"));
    }
    dir
}

#[test]
fn aio_directory_keeps_par_beside_the_new_catalog() {
    let (top_files, aio_files) = catalog_yaml_files();
    assert_eq!(sorted_file_names(&top_files), TOP_LEVEL_YAMLS);
    assert_eq!(sorted_file_names(&aio_files), AIO_YAMLS);

    let aio_bytes = read_by_name(&aio_files);
    let top_bytes = read_top_level(&top_files, &aio_bytes);
    for name in TOP_LEVEL_YAMLS {
        assert_eq!(
            top_bytes.get(*name),
            aio_bytes.get(*name),
            "{name} must match aio/{name}"
        );
    }

    let top_dir = stage_named_bytes(&top_bytes);
    let aio_dir = stage_named_bytes(&aio_bytes);

    let top = load_configs(top_dir.path()).expect("top-level processes.d parses");
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
    let par_control = top
        .processes
        .iter()
        .find(|process| process.name == "datadog-agent-par-control")
        .expect("par-control");
    assert_eq!(
        par_control.config.condition_path_exists.as_deref(),
        Some(PAR_CONTROL),
        "par-control must be skipped when the FIPS image omits the binary"
    );

    let catalog = load_configs(aio_dir.path()).expect("aio directory parses");
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
            "datadog-agent-par",
            "datadog-agent-par-control",
            "datadog-agent-process",
            "datadog-agent-security",
            "datadog-agent-sysprobe",
            "datadog-agent-trace",
        ]
    );

    let executor = catalog
        .processes
        .iter()
        .find(|process| process.name == "datadog-agent-action-executor")
        .expect("action-executor");
    assert!(
        !executor.config.auto_start,
        "action-executor stays on demand"
    );

    // Both read the cert path from DD_PM_IPC_CERT_FILE_PATH, not the Agent setting.
    for name in ["datadog-agent-par-control", "datadog-agent-action-executor"] {
        let process = catalog
            .processes
            .iter()
            .find(|process| process.name == name)
            .unwrap_or_else(|| panic!("{name}"));
        let flag = process
            .config
            .args
            .iter()
            .position(|arg| arg == "--ipc-cert-file")
            .unwrap_or_else(|| panic!("{name} has no --ipc-cert-file"));
        assert_eq!(
            process.config.args.get(flag + 1).map(String::as_str),
            Some("${DD_PM_IPC_CERT_FILE_PATH}"),
            "{name} ipc cert argument"
        );
    }
    for (name, body) in &aio_bytes {
        assert!(
            !String::from_utf8_lossy(body).contains("${DD_IPC_CERT_FILE_PATH}"),
            "{name} must not read the Agent setting name"
        );
    }
}

#[test]
fn procmgr_all_in_one_entrypoint_runs_cont_init_then_dd_procmgrd() {
    let script = ENTRYPOINT_PROCMGR_ALL_IN_ONE;
    assert!(script.contains("set -euo pipefail"));
    // s6 runs every regular file by its shebang. The entrypoint stages executable
    // copies because shipped and customer scripts are mode 0644.
    assert!(
        !script.contains("/etc/cont-init.d/*.sh"),
        "a .sh-only glob skips customer scripts that have another suffix"
    );
    let code_lines: Vec<&str> = script
        .lines()
        .map(str::trim)
        .filter(|line| !line.is_empty() && !line.starts_with('#'))
        .collect();
    assert!(
        code_lines
            .iter()
            .all(|line| !line.split_whitespace().any(|word| word == "bash")),
        "run every regular file by its own shebang, not bash"
    );
    let pos = |needle: &str| {
        code_lines
            .iter()
            .position(|line| *line == needle)
            .unwrap_or_else(|| panic!("missing line: {needle}"))
    };
    let mapfile_line =
        r#"mapfile -d '' inits < <(printf '%s\0' /etc/cont-init.d/* | LC_ALL=C sort -z)"#;
    let lc_all: Vec<_> = code_lines
        .iter()
        .copied()
        .filter(|line| line.contains("LC_ALL"))
        .collect();
    assert_eq!(
        lc_all,
        [mapfile_line],
        "LC_ALL is only the sort prefix, so dd-procmgrd does not inherit it"
    );
    // The loops are compared as one contiguous slice, so the generic fi and done lines
    // stay attached to the bodies they close.
    let staging_cleanup =
        r#"if [[ "$copy_dir" != "$primary" ]]; then rm -rf "$copy_dir"; trap - EXIT TERM INT; fi"#;
    let steps = [
        "primary=/var/run/s6/procmgr-cont-init.d",
        r#"if rm -rf "$primary" 2>/dev/null && mkdir "$primary" 2>/dev/null; then"#,
        "copy_dir=$primary",
        r#"elif copy_dir=$(mktemp -d /opt/datadog-agent/run/procmgr-cont-init.d.XXXXXX 2>/dev/null); then"#,
        r#"trap 'rm -rf "$copy_dir"' EXIT"#,
        r#"trap 'rm -rf "$copy_dir"; exit 143' TERM INT"#,
        "else",
        r#"echo "cannot stage /etc/cont-init.d: neither /var/run/s6 nor /opt/datadog-agent/run is writable" >&2"#,
        "exit 1",
        "fi",
        r#"printf '===== STAGING cont-init in %s =====\n' "$copy_dir""#,
        mapfile_line,
        r#"wait "$!""#,
        "selected=()",
    ];
    let positions: Vec<_> = steps.iter().map(|needle| pos(needle)).collect();
    for (index, pair) in positions.windows(2).enumerate() {
        assert!(
            pair[0] < pair[1],
            "`{}` must come before `{}`",
            steps[index],
            steps[index + 1]
        );
    }
    let loops = [
        r#"for init in "${inits[@]}"; do"#,
        r#"if [[ -f "$init" ]]; then"#,
        r#"copy="$copy_dir/${init##*/}""#,
        r#"cp -L -- "$init" "$copy""#,
        r#"chmod 0755 "$copy""#,
        r#"selected+=("$init")"#,
        "fi",
        "done",
        r#"for init in "${selected[@]}"; do"#,
        r#"printf '===== RUNNING %s =====\n' "$init""#,
        r#""$copy_dir/${init##*/}""#,
        "done",
    ];
    let stage_at = pos(loops[0]);
    let loops_end = stage_at + loops.len();
    assert!(
        positions.last().copied().unwrap() < stage_at,
        "pick the copy dir before either loop"
    );
    assert_eq!(
        &code_lines[stage_at..loops_end],
        &loops[..],
        "the copy loop, including fi and done, must finish before the run loop"
    );
    assert_eq!(
        code_lines[loops_end], staging_cleanup,
        "remove the fallback copy dir right after the run loop, because exec skips the EXIT trap"
    );
    assert!(
        loops_end < pos("exec /opt/datadog-agent/embedded/bin/dd-procmgrd"),
        "both loops and the cleanup must finish before dd-procmgrd"
    );
    assert!(
        !script.contains("mkdir -p"),
        "mkdir -p would accept a leftover directory this UID cannot clean"
    );
    assert!(script.contains("DD_PM_CONFIG_DIR=/opt/datadog-agent/processes.d/aio"));
    assert!(script.contains(
        r#"DD_PM_SOCKET_PATH="${DD_PM_SOCKET_PATH:-/opt/datadog-agent/run/dd-procmgrd.sock}""#
    ));
    assert!(
        !script.contains("IPC_CERT_FILE_PATH"),
        "an exported DD_IPC_CERT_FILE_PATH would override ipc_cert_file_path in datadog.yaml, \
         dd-procmgrd resolves the certificate path itself"
    );
    assert!(
        !script.contains("DD_PM_INHERIT_ENV"),
        "the entrypoint must not set DD_PM_INHERIT_ENV"
    );
}

#[test]
fn split_entrypoint_leaves_the_ipc_cert_path_to_dd_procmgrd() {
    let script = ENTRYPOINT_PRIVATE_ACTION_RUNNER;
    assert!(
        !script.contains("IPC_CERT_FILE_PATH"),
        "an exported DD_IPC_CERT_FILE_PATH would override ipc_cert_file_path in datadog.yaml, \
         dd-procmgrd resolves the certificate path itself"
    );
    assert!(script.contains(r#"export DD_PM_CONFIG_DIR="${processes_dir}""#));
    assert!(script.contains(r#"exec "${procmgrd}""#));
}

#[test]
fn default_entrypoint_still_execs_init() {
    assert!(ENTRYPOINT_DEFAULT.contains("exec /init"));
    assert!(
        !ENTRYPOINT_DEFAULT.contains("dd-procmgrd"),
        "_default must keep exec /init and must not exec dd-procmgrd"
    );
}

#[test]
fn s6_all_in_one_entrypoint_execs_init() {
    assert!(ENTRYPOINT_S6_ALL_IN_ONE.contains("exec /init"));
    assert!(
        !ENTRYPOINT_S6_ALL_IN_ONE.contains("dd-procmgrd"),
        "s6-all-in-one pins s6 and must not exec dd-procmgrd"
    );
}

#[test]
fn procmgr_all_in_one_entrypoint_is_installed_executable() {
    assert!(
        AGENT_DOCKERFILE
            .lines()
            .any(|line| { line.trim() == "COPY --chmod=755 entrypoint.d /opt/entrypoints" })
    );
    assert!(
        AGENT_DOCKERFILE.lines().any(|line| {
            line.trim() == r#"VOLUME ["/var/run/s6", "/opt/datadog-agent/run", "/var/log/datadog"]"#
        }),
        "Docker --read-only only writes VOLUME paths, and the all-in-one socket lives in /opt/datadog-agent/run"
    );
    assert!(
        ENTRYPOINT_LAUNCHER.contains(r#"[[ -x "$ENTRYPOINT_PATH/$ENTRYPOINT" ]]"#),
        "entrypoint.sh must exec only an executable entrypoint"
    );
}
