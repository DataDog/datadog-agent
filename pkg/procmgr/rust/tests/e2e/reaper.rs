// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Linux subreaper: orphans of supervised processes are adopted and reaped.

use crate::helpers::{ProcessExpect, TestEnv, pid_is_alive, wait_for_pid_gone};
use std::path::Path;
use std::time::{Duration, Instant};

const WAIT: Duration = Duration::from_secs(15);

fn wait_for_grandchild_pid(root: &Path) -> u32 {
    let pidfile = root.join("grandchild.pid");
    let deadline = Instant::now() + WAIT;
    loop {
        if let Ok(raw) = std::fs::read_to_string(&pidfile)
            && let Ok(pid) = raw.trim().parse()
        {
            return pid;
        }
        assert!(
            Instant::now() < deadline,
            "grandchild pid file never appeared at {}",
            pidfile.display()
        );
        std::thread::sleep(Duration::from_millis(50));
    }
}

fn parent_pid(pid: u32) -> Option<u32> {
    let stat = std::fs::read_to_string(format!("/proc/{pid}/stat")).ok()?;
    // `pid (comm) state ppid ...`; comm may contain spaces/parens.
    let after_comm = stat.rsplit_once(')')?.1;
    after_comm.split_whitespace().nth(1)?.parse().ok()
}

#[test]
fn orphaned_grandchild_is_adopted_and_reaped() {
    let procmgr = TestEnv::new().with_process("orphan_grandchild").start();
    let daemon_pid = procmgr.daemon_pid();
    let grandchild = wait_for_grandchild_pid(procmgr.env_root());

    let deadline = Instant::now() + WAIT;
    while parent_pid(grandchild) != Some(daemon_pid) {
        assert!(
            Instant::now() < deadline && pid_is_alive(grandchild),
            "grandchild {grandchild} was not adopted by the daemon {daemon_pid} (ppid {:?})",
            parent_pid(grandchild)
        );
        std::thread::sleep(Duration::from_millis(20));
    }

    assert!(
        wait_for_pid_gone(grandchild, WAIT),
        "orphan {grandchild} stayed a zombie under the daemon"
    );
    assert!(
        procmgr.daemon().wait_for_log(
            &format!("reaped orphaned child pid={grandchild}"),
            Duration::from_secs(5)
        ),
        "reaper did not log the orphan it collected: {:?}",
        procmgr.daemon().captured_logs()
    );

    procmgr
        .require_list()
        .assert_process_state("orphan_grandchild", ProcessExpect::Running);
}

#[test]
fn supervised_exit_status_is_not_stolen_by_the_reaper() {
    let procmgr = TestEnv::new().with_process("exit_fail").start();
    procmgr.assert_process_state_within("exit_fail", ProcessExpect::Failed);
    procmgr.require_list().assert_last_exit_code("exit_fail", 1);
}
