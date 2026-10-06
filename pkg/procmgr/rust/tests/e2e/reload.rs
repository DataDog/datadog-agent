// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::helpers::{ProcessExpect, ReloadExpect, TestEnv, write_config};
use dd_procmgrd::test_helpers;

#[test]
fn reload_unchanged_leaves_running_process() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    procmgr.assert_reload_matches(ReloadExpect {
        unchanged: Some(vec!["sleeper".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        modified: Some(vec![]),
        preserve_running_pids: vec!["sleeper".into()],
    });
}

#[test]
fn reload_adds_fixture_to_catalog() {
    let procmgr = TestEnv::new().start();
    procmgr.require_list().assert_empty();

    procmgr.install_fixture("sleeper");
    procmgr.assert_reload_matches(ReloadExpect {
        added: Some(vec!["sleeper".into()]),
        ..Default::default()
    });
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper", ProcessExpect::Running);
}

#[test]
fn reload_removes_deleted_yaml_from_catalog() {
    let procmgr = TestEnv::new()
        .with_process("sleeper")
        .with_process("sleeper_idle")
        .start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(2);
    list.assert_process_state("sleeper", ProcessExpect::Running);
    list.assert_process_state("sleeper_idle", ProcessExpect::Created);

    procmgr.remove_process_yaml("sleeper_idle");
    procmgr.assert_reload_matches(ReloadExpect {
        removed: Some(vec!["sleeper_idle".into()]),
        unchanged: Some(vec!["sleeper".into()]),
        added: Some(vec![]),
        modified: Some(vec![]),
        preserve_running_pids: vec!["sleeper".into()],
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_absent("sleeper_idle");
}

#[test]
fn reload_remove_only_empties_catalog() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let list = procmgr.require_list();
    list.assert_len(1);
    let old_pid = procmgr.process("sleeper").expect("sleeper").pid;

    procmgr.remove_process_yaml("sleeper");
    procmgr.assert_reload_matches(ReloadExpect {
        removed: Some(vec!["sleeper".into()]),
        added: Some(vec![]),
        modified: Some(vec![]),
        unchanged: Some(vec![]),
        ..Default::default()
    });

    procmgr.assert_pid_gone(old_pid);
    procmgr.require_list().assert_empty();
}

#[test]
fn reload_adds_no_auto_start_stays_created() {
    let procmgr = TestEnv::new().start();
    procmgr.require_list().assert_empty();

    procmgr.install_fixture("sleeper_idle");
    procmgr.assert_reload_matches(ReloadExpect {
        added: Some(vec!["sleeper_idle".into()]),
        ..Default::default()
    });
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper_idle", ProcessExpect::Created);
}

#[test]
fn reload_modify_respawns_with_new_pid() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let count_before = procmgr.require_list().len();
    let old_pid = procmgr.process("sleeper").expect("sleeper").pid;

    procmgr.overwrite_with_fixture("sleeper", "sleeper_long");

    procmgr.assert_reload_matches(ReloadExpect {
        modified: Some(vec!["sleeper".into()]),
        added: Some(vec![]),
        removed: Some(vec![]),
        unchanged: Some(vec![]),
        ..Default::default()
    });

    let count_after = procmgr.require_list().len();
    assert_eq!(
        count_before, count_after,
        "modify reload should not change catalog size"
    );
    procmgr.assert_process_pid_changed("sleeper", old_pid);
}

#[test]
fn reload_remove_and_add_swaps_catalog() {
    let procmgr = TestEnv::new().with_process("sleeper").start();
    procmgr
        .wait_for_process_running("sleeper")
        .expect("expected sleeper running");
    let old_pid = procmgr.process("sleeper").expect("sleeper").pid;

    procmgr.remove_process_yaml("sleeper");
    procmgr.install_fixture("sleeper_idle");

    procmgr.assert_reload_matches(ReloadExpect {
        removed: Some(vec!["sleeper".into()]),
        added: Some(vec!["sleeper_idle".into()]),
        modified: Some(vec![]),
        unchanged: Some(vec![]),
        ..Default::default()
    });

    procmgr.assert_pid_gone(old_pid);
    let list = procmgr.require_list();
    list.assert_len(1);
    list.assert_process_state("sleeper_idle", ProcessExpect::Created);
}

#[test]
fn test_cli_reload_text_modified() {
    let env = TestEnv::new().with_process("sleeper").start();
    env.overwrite_with_fixture("sleeper", "sleeper_long");

    env.cli_reload()
        .assert_success()
        .assert_field("Modified", "sleeper");
}

#[test]
fn test_cli_reload_text_added_removed() {
    let env = TestEnv::new().with_process("sleeper").start();
    env.remove_process_yaml("sleeper");
    env.install_fixture("sleeper_idle");

    env.cli_reload()
        .assert_success()
        .assert_field("Added", "sleeper_idle")
        .assert_field("Removed", "sleeper");
}

#[test]
fn test_cli_reload_json() {
    let env = TestEnv::new()
        .with_config("existing", test_helpers::sleep_config_yaml())
        .start();

    env.daemon().wait_for_log_default("[existing] spawned");

    write_config(env.config_dir(), "added", test_helpers::sleep_config_yaml());

    let out = env.cli_reload_json();
    out.assert_success();
    let json = out.stdout_json();

    let added = json["added"].as_array().expect("added should be an array");
    assert!(
        added.iter().any(|v| v.as_str() == Some("added")),
        "expected 'added' in added array: {json}"
    );

    let unchanged = json["unchanged"]
        .as_array()
        .expect("unchanged should be an array");
    assert!(
        unchanged.iter().any(|v| v.as_str() == Some("existing")),
        "expected 'existing' in unchanged array: {json}"
    );

    assert!(json["removed"].as_array().expect("array").is_empty());
    assert!(json["modified"].as_array().expect("array").is_empty());
}
