// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers;

#[cfg(unix)]
fn render_ddot_template(install_dir: &str, pid_dir: &str) -> Option<String> {
    let tmpl_path = std::path::PathBuf::from(option_env!("DDOT_TEMPLATE_PATH")?);
    let tmpl = std::fs::read_to_string(&tmpl_path)
        .unwrap_or_else(|e| panic!("failed to read {}: {e}", tmpl_path.display()));
    let rendered = tmpl
        .replace("{{.InstallDir}}", install_dir)
        .replace("{{.PIDDir}}", pid_dir);
    Some(
        rendered
            .lines()
            .filter(|line| !line.contains("{{"))
            .collect::<Vec<_>>()
            .join("\n"),
    )
}

#[cfg(unix)]
#[test]
fn test_ddot_template_starts_with_env_and_optional_envfile() {
    let dir = tempfile::tempdir().unwrap();

    let install_dir = dir.path().join("install");
    let bin_dir = install_dir.join("ext/ddot/embedded/bin");
    std::fs::create_dir_all(&bin_dir).unwrap();

    let conf_dir = dir.path().join("conf");
    std::fs::create_dir_all(&conf_dir).unwrap();
    let fleet_policies_dir = conf_dir.join("managed/datadog-agent/stable");

    let script = bin_dir.join("otel-agent");
    std::fs::write(
        &script,
        format!(
            concat!(
                "#!/bin/sh\n",
                "if [ \"$DD_FLEET_POLICIES_DIR\" = \"{expected}\" ]; then\n",
                "  exit 0\n",
                "else\n",
                "  echo \"DD_FLEET_POLICIES_DIR=$DD_FLEET_POLICIES_DIR\" >&2\n",
                "  exit 1\n",
                "fi\n",
            ),
            expected = fleet_policies_dir.to_str().unwrap(),
        ),
    )
    .unwrap();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(&script, std::fs::Permissions::from_mode(0o755)).unwrap();
    }

    let pid_dir = dir.path().join("pid");
    std::fs::create_dir_all(pid_dir.join("run")).unwrap();

    let config_dir = dir.path().join("processes.d");
    std::fs::create_dir_all(&config_dir).unwrap();

    let Some(yaml) = render_ddot_template(install_dir.to_str().unwrap(), pid_dir.to_str().unwrap())
    else {
        eprintln!("DDOT_TEMPLATE_PATH not set at compile time, skipping");
        return;
    };
    std::fs::write(config_dir.join("datadog-agent-ddot.yaml"), &yaml).unwrap();

    let sock = dir.path().join("daemon.sock");
    let mut daemon = helpers::DaemonHandle::start_with_env(
        &config_dir,
        &sock,
        &[
            ("DD_CONF_DIR", conf_dir.to_str().unwrap()),
            ("DD_INSTALL_DIR", install_dir.to_str().unwrap()),
            ("DD_PID_DIR", pid_dir.to_str().unwrap()),
        ],
    );
    assert!(
        daemon.wait_for_log_default(
            "[datadog-agent-ddot] optional environment file not found, skipping"
        ),
        "daemon should skip the missing optional env file for ddot"
    );
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] spawned (pid="),
        "ddot process should be spawned with the otel-agent binary"
    );
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] exited with exit status: 0"),
        "ddot process should exit 0 (DD_FLEET_POLICIES_DIR env var was set correctly)"
    );
    assert!(
        daemon.wait_for_log_default(
            "[datadog-agent-ddot] exit does not match restart policy, not restarting"
        ),
        "on-failure restart should not trigger on exit 0"
    );

    let status = daemon.stop();
    assert!(status.success());
}

#[cfg(unix)]
#[test]
fn test_ddot_template_skipped_when_binary_missing() {
    let dir = tempfile::tempdir().unwrap();
    let config_dir = dir.path().join("processes.d");
    std::fs::create_dir_all(&config_dir).unwrap();

    let Some(yaml) = render_ddot_template("/nonexistent/install", "/nonexistent/pid") else {
        eprintln!("DDOT_TEMPLATE_PATH not set at compile time, skipping");
        return;
    };
    std::fs::write(config_dir.join("datadog-agent-ddot.yaml"), &yaml).unwrap();

    let sock = dir.path().join("daemon.sock");
    let mut daemon = helpers::DaemonHandle::start(&config_dir, &sock);
    assert!(
        daemon.wait_for_log_default("[datadog-agent-ddot] condition_path_exists not met"),
        "daemon should skip ddot when otel-agent binary is missing"
    );
    assert_eq!(
        daemon.count_log_matches("[datadog-agent-ddot] spawned"),
        0,
        "ddot should NOT be spawned when condition_path_exists is not met"
    );

    let status = daemon.stop();
    assert!(status.success());
}
