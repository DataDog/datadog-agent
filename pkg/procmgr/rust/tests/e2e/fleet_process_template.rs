// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Round-trip coverage for the Windows processes.d entry the fleet installer ships for
//! process-agent. The template is consumed as the file a human edits, rendered the way the
//! installer renders it, and fed to a real daemon.
//!
//! `PROCESS_TEMPLATE_PATH` is a Bazel `rootpath`, so `option_env!` is `None` under plain
//! `cargo test` and every test here skips. To run them locally, pass the path at compile
//! time:
//!
//! ```text
//! cd pkg/procmgr/rust
//! PROCESS_TEMPLATE_PATH=$PWD/../../fleet/installer/packages/embedded/tmpl/datadog-agent-process-windows.yaml.tmpl \
//!   cargo test -p dd-procmgrd --test e2e --features test-helpers fleet_process_template
//! ```

use crate::helpers::{DescribeExpect, ProcessExpect, StatusProcessesCount, TestEnv};
use std::path::{Path, PathBuf};

/// Catalog name of the shipped entry, from `datadog-agent-process.yaml` in `processes.d`.
/// It must stay in sync with `dd_procmgrd::spawn::DATADOG_AGENT_PROCESS`, which is what
/// selects the Windows privileged spawn profile for this process.
const PROCESS_NAME: &str = "datadog-agent-process";

/// The legacy SCM service definition the template transcribes. Parsed rather than
/// duplicated so that editing the SCM key list without editing the template fails here.
const SCM_SERVICES_GO: &str =
    include_str!("../../../../../cmd/agent/subcommands/run/dependent_services_windows.go");

/// Gate keys all absent: a default install has them true, but nothing in these tests may
/// depend on that, since `auto_start: false` means the gate is never evaluated.
const EMPTY_AGENT_YAML: &str = "api_key: 0000001\n";
const EMPTY_SYSPROBE_YAML: &str = "# no modules enabled\n";

#[test]
fn fleet_process_template_parses_into_catalog() {
    let env = TestEnv::new();
    let layout = InstallLayout::create(env.env_root(), EMPTY_AGENT_YAML, EMPTY_SYSPROBE_YAML);
    let Some(yaml) = layout.render_template() else {
        return;
    };
    let procmgr = env.with_config(PROCESS_NAME, &yaml).start();

    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state(PROCESS_NAME, ProcessExpect::Created);

    procmgr.assert_describe_matches(
        PROCESS_NAME,
        DescribeExpect {
            name: Some(PROCESS_NAME.into()),
            state: Some("Created".into()),
            description: Some("Datadog Process Agent".into()),
            command: Some(layout.binary()),
            args: Some(vec!["--cfgpath".into(), layout.agent_yaml()]),
            condition_path_exists: Some(layout.binary()),
            restart_policy: Some("on-failure".into()),
            auto_start: Some(false),
            ..Default::default()
        },
    );
}

#[test]
fn fleet_process_template_declares_legacy_scm_gate() {
    let dir = tempfile::tempdir().expect("tempdir");
    let layout = InstallLayout::create(dir.path(), EMPTY_AGENT_YAML, EMPTY_SYSPROBE_YAML);
    let Some(yaml) = layout.render_template() else {
        return;
    };
    let catalog_dir = dir.path().join("processes.d");
    std::fs::create_dir_all(&catalog_dir).expect("mkdir processes.d");
    std::fs::write(catalog_dir.join(format!("{PROCESS_NAME}.yaml")), &yaml).expect("write yaml");

    let definitions = dd_procmgrd::config::load_configs(&catalog_dir).expect("load catalog");
    assert_eq!(definitions.len(), 1, "expected exactly one process");
    let config = &definitions[0].config;

    let (core_keys, sysprobe_keys) = scm_process_keys();
    let gate = &config.condition_config_any;
    assert_eq!(
        gate.len(),
        2,
        "the gate must split across datadog.yaml and system-probe.yaml, got {gate:?}"
    );
    assert_eq!(gate[0].path, layout.agent_yaml());
    assert_eq!(
        sorted(&gate[0].keys),
        core_keys,
        "drift from the SCM coreConf keys"
    );
    assert_eq!(gate[1].path, layout.sysprobe_yaml());
    assert_eq!(
        sorted(&gate[1].keys),
        sysprobe_keys,
        "drift from the SCM sysprobeConf keys"
    );

    // Supervision knobs the catalog entry carries into A12, when it starts spawning.
    assert_eq!(config.restart.to_string(), "on-failure");
    assert_eq!(config.restart_sec, Some(2.0));
    assert_eq!(config.start_limit_interval_sec, Some(10));
    assert_eq!(config.start_limit_burst, Some(5));
    // The Windows privileged spawn profile rejects anything but inherit or null.
    assert_eq!(config.stdout, "inherit");
    assert_eq!(config.stderr, "inherit");
}

#[test]
fn fleet_process_template_stays_created_with_auto_start_false() {
    let env = TestEnv::new();
    let layout = InstallLayout::create(
        env.env_root(),
        "process_config:\n  process_collection:\n    enabled: true\n",
        "network_config:\n  enabled: true\n",
    );
    let Some(yaml) = layout.render_template() else {
        return;
    };
    let procmgr = env.with_config(PROCESS_NAME, &yaml).start();

    // `start()` returns once the daemon is ready, which is after boot auto-start has run.
    let status = procmgr.require_status();
    status.assert_ready();
    status.assert_processes_count(StatusProcessesCount {
        total: Some(1),
        created: Some(1),
        running: Some(0),
        failed: Some(0),
        exited: Some(0),
        ..Default::default()
    });
    procmgr
        .require_list()
        .assert_process_state(PROCESS_NAME, ProcessExpect::Created);
}

/// The two roots the Windows installer substitutes into the template, laid out on disk the
/// way an install does: `process-agent.exe` under the install root, the gated config files
/// under the data root.
struct InstallLayout {
    install_dir: PathBuf,
    etc_dir: PathBuf,
}

impl InstallLayout {
    fn create(root: &Path, agent_yaml: &str, sysprobe_yaml: &str) -> Self {
        let install_dir = root.join("install");
        let bin_dir = install_dir.join("bin/agent");
        std::fs::create_dir_all(&bin_dir).expect("mkdir bin/agent");
        std::fs::write(bin_dir.join("process-agent.exe"), b"").expect("touch process-agent.exe");

        let etc_dir = root.join("etc");
        std::fs::create_dir_all(&etc_dir).expect("mkdir etc");
        std::fs::write(etc_dir.join("datadog.yaml"), agent_yaml).expect("write datadog.yaml");
        std::fs::write(etc_dir.join("system-probe.yaml"), sysprobe_yaml)
            .expect("write system-probe.yaml");

        Self {
            install_dir,
            etc_dir,
        }
    }

    fn binary(&self) -> String {
        format!("{}/bin/agent/process-agent.exe", self.install_dir.display())
    }

    fn agent_yaml(&self) -> String {
        format!("{}/datadog.yaml", self.etc_dir.display())
    }

    fn sysprobe_yaml(&self) -> String {
        format!("{}/system-probe.yaml", self.etc_dir.display())
    }

    /// Render the shipped `.tmpl` the way `renderConfig` does at install time, or `None`
    /// when the template path was not wired in at compile time.
    fn render_template(&self) -> Option<String> {
        let Some(tmpl_path) = option_env!("PROCESS_TEMPLATE_PATH") else {
            eprintln!("PROCESS_TEMPLATE_PATH not set at compile time, skipping");
            return None;
        };
        let tmpl_path = PathBuf::from(tmpl_path);
        let tmpl = std::fs::read_to_string(&tmpl_path)
            .unwrap_or_else(|e| panic!("failed to read {}: {e}", tmpl_path.display()));
        let rendered = tmpl
            .replace("{{.InstallDir}}", &self.install_dir.display().to_string())
            .replace("{{.EtcDir}}", &self.etc_dir.display().to_string());
        assert!(
            !rendered.contains("{{"),
            "the installer only substitutes __PROCESS_INSTALL_ROOT__ and __PROCESS_ETC_ROOT__, so \
             the template must use no other placeholder:\n{rendered}"
        );
        Some(rendered)
    }
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
