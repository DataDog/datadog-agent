// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Shared plumbing for the checks on the Windows processes.d entries the fleet installer
//! ships: rendering a template the way the installer does, and reading the legacy SCM
//! service definitions those entries transcribe.

use crate::config::{ProcessConfig, load_configs};
use std::path::Path;

/// The legacy SCM service definitions. Parsed rather than duplicated so that editing an
/// SCM key list without editing the matching template fails a test.
const SCM_SERVICES_GO: &str =
    include_str!("../../../../cmd/agent/subcommands/run/dependent_services_windows.go");

pub(crate) const INSTALL_DIR: &str = "C:/Program Files/Datadog/Datadog Agent";

pub(crate) fn write_gated_files(etc: &Path, agent_yaml: &str, sysprobe_yaml: &str) {
    std::fs::write(etc.join("datadog.yaml"), agent_yaml).expect("write datadog.yaml");
    std::fs::write(etc.join("system-probe.yaml"), sysprobe_yaml).expect("write system-probe.yaml");
}

/// Render a shipped `.tmpl` the way `renderConfig` does at install time, with `etc` as
/// the data root, and load it as `<name>.yaml` through the same catalog loader the
/// daemon uses.
pub(crate) fn load_template(template: &str, name: &str, etc: &Path) -> ProcessConfig {
    let rendered = template
        .replace("{{.InstallDir}}", INSTALL_DIR)
        .replace("{{.EtcDir}}", &etc.display().to_string());
    assert!(
        !rendered.contains("{{"),
        "the installer only substitutes InstallDir and EtcDir, so the template must use no \
         other placeholder:\n{rendered}"
    );

    let catalog = tempfile::tempdir().expect("tempdir");
    std::fs::write(catalog.path().join(format!("{name}.yaml")), rendered).expect("write yaml");
    let mut definitions = load_configs(catalog.path()).expect("load catalog");
    assert_eq!(definitions.len(), 1, "expected exactly one process");
    definitions.remove(0).config
}

pub(crate) fn sorted(keys: &[String]) -> Vec<String> {
    let mut sorted = keys.to_vec();
    sorted.sort();
    sorted
}

/// The `configKeys` of the legacy SCM `service` definition, sorted and split by the config
/// file each key is read from: `(coreConf keys, sysprobeConf keys)`. A Go map literal has
/// no meaningful order, so only the sets are compared. Both SCM services these templates
/// transcribe read from both files, so an empty half means the parse went wrong.
pub(crate) fn scm_service_keys(service: &str) -> (Vec<String>, Vec<String>) {
    let marker = format!("name: \"{service}\",");
    let definition = SCM_SERVICES_GO
        .find(&marker)
        .map(|start| &SCM_SERVICES_GO[start..])
        .unwrap_or_else(|| {
            panic!("no `{marker}` service definition in dependent_services_windows.go")
        });
    let map = definition
        .find("configKeys: map[string]model.Reader{")
        .map(|start| &definition[start..])
        .unwrap_or_else(|| panic!("the SCM {service} service has no configKeys map"));
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
        "parsed an implausible SCM key split for {service}: core={core:?} sysprobe={sysprobe:?}"
    );
    (sorted(&core), sorted(&sysprobe))
}
