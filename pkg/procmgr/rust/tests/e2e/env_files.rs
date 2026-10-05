// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::TestEnv;
use dd_procmgrd::test_helpers;

#[cfg(unix)]
#[test]
fn test_cli_environment_file_loading() {
    let env = TestEnv::new();
    let env_file = env.config_dir().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    let env = env
        .with_config(
            "env-test",
            &format!(
                concat!(
                    "command: /bin/sh\n",
                    "args:\n",
                    "  - '-c'\n",
                    "  - 'exit $(test \"$MY_VAR\" = \"from_file\" && echo 0 || echo 1)'\n",
                    "environment_file: {}\n",
                    "restart: never\n",
                ),
                env_file.display()
            ),
        )
        .start();

    env.daemon().wait_for_log_default("[env-test] exited with");

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[cfg(unix)]
#[test]
fn test_cli_env_overrides_environment_file() {
    let env = TestEnv::new();
    let env_file = env.config_dir().join("test.env");
    std::fs::write(&env_file, "MY_VAR=from_file\n").unwrap();

    let env = env
        .with_config(
            "override-test",
            &format!(
                concat!(
                    "command: /bin/sh\n",
                    "args:\n",
                    "  - '-c'\n",
                    "  - 'exit $(test \"$MY_VAR\" = \"overridden\" && echo 0 || echo 1)'\n",
                    "environment_file: {}\n",
                    "env:\n",
                    "  MY_VAR: overridden\n",
                    "restart: never\n",
                ),
                env_file.display()
            ),
        )
        .start();

    env.daemon()
        .wait_for_log_default("[override-test] exited with");

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[cfg(unix)]
#[test]
fn test_cli_child_does_not_inherit_parent_env() {
    let env = TestEnv::new()
        .with_config(
            "clean-env",
            concat!(
                "command: /bin/sh\n",
                "args:\n",
                "  - '-c'\n",
                "  - 'test -z \"$HOME\" && exit 0 || exit 1'\n",
                "restart: never\n",
            ),
        )
        .start();

    env.daemon().wait_for_log_default("[clean-env] exited with");

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}

#[test]
fn test_cli_optional_environment_file_skipped_when_missing() {
    let (sh, flag) = test_helpers::shell_cmd();
    let env = TestEnv::new()
        .with_config(
            "opt-env",
            &test_helpers::cmd_yaml(
                sh,
                &[flag.to_string(), "exit 0".to_string()],
                "environment_file: -/nonexistent/env\nrestart: never\n",
            ),
        )
        .start();

    env.daemon()
        .wait_for_log_default("optional environment file not found, skipping");
    env.daemon().wait_for_log_default("[opt-env] exited with");

    let json = env.cli_list_json().stdout_json();
    assert_eq!(json[0]["state"], "Exited");
    assert_eq!(json[0]["last_exit_code"], 0);
}
