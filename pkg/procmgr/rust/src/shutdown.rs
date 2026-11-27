// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use crate::process::ManagedProcess;
#[cfg(test)]
use crate::process::test_exit_channel;
use std::collections::{HashMap, HashSet};
use tokio::task::{Id, JoinSet};

/// Stops every process in `order`. A process is signaled only after its dependents
/// (`dependents[idx]`, limited to `order`) have finished stopping. Ready processes stop
/// concurrently, each force-killed at its own deadline.
///
/// When some processes wait only on each other (a dependency cycle), it drops only the edge
/// that closes the cycle, even while unrelated stops are still in progress.
pub async fn shutdown_ordered(
    processes: &mut [ManagedProcess],
    order: &[usize],
    dependents: &[Vec<usize>],
) {
    let scheduled: HashSet<usize> = order.iter().copied().collect();
    let mut blockers: HashMap<usize, HashSet<usize>> = order
        .iter()
        .map(|&idx| {
            let waits_for = dependents
                .get(idx)
                .into_iter()
                .flatten()
                .copied()
                .filter(|&dep| dep != idx && scheduled.contains(&dep))
                .collect();
            (idx, waits_for)
        })
        .collect();
    let mut signaled: HashSet<usize> = HashSet::new();
    let mut finished: HashSet<usize> = HashSet::new();
    let mut waits: JoinSet<usize> = JoinSet::new();
    let mut wait_owner: HashMap<Id, usize> = HashMap::new();

    loop {
        // A process without a wait finishes at once and can unblock others.
        let mut progressed = true;
        while progressed {
            progressed = false;
            for &idx in order {
                if signaled.contains(&idx) || !blockers[&idx].iter().all(|b| finished.contains(b)) {
                    continue;
                }
                signaled.insert(idx);
                processes[idx].request_stop();
                match processes[idx].take_stop_wait() {
                    Some(wait) => {
                        let handle = waits.spawn(async move {
                            wait.run().await;
                            idx
                        });
                        wait_owner.insert(handle.id(), idx);
                    }
                    None => {
                        processes[idx].finish_stop();
                        finished.insert(idx);
                        progressed = true;
                    }
                }
            }
        }

        let stuck = stuck_set(order, &signaled, &blockers, &finished);
        if let Some(start) = order.iter().copied().find(|idx| stuck.contains(idx)) {
            match cycle_edge(start, order, &blockers, &stuck) {
                Some((from, to)) => {
                    log::warn!(
                        "[{}] dependency cycle among stopped processes, will not wait for dependent [{}]",
                        processes[from].name(),
                        processes[to].name()
                    );
                    blockers.entry(from).or_default().remove(&to);
                }
                None => {
                    // Unreachable. Still guarantees progress if it ever happens.
                    debug_assert!(false, "stuck process without a cycle edge");
                    blockers.entry(start).or_default().clear();
                }
            }
            continue;
        }

        if waits.is_empty() {
            debug_assert!(
                order.iter().all(|idx| signaled.contains(idx)),
                "shutdown ended with unsignaled processes"
            );
            break;
        }

        match waits.join_next().await {
            Some(Ok(idx)) => {
                processes[idx].finish_stop();
                finished.insert(idx);
            }
            Some(Err(error)) => {
                let idx = wait_owner[&error.id()];
                log::warn!(
                    "[{}] stop wait failed: {error}; leaving its state unfinalized",
                    processes[idx].name()
                );
                finished.insert(idx);
            }
            None => unreachable!("waits checked non-empty above"),
        }
    }
}

/// Unsignaled processes whose unfinished dependents are all unsignaled too. Each one waits
/// on another member, so the set holds every cycle that blocks shutdown. A process waiting
/// on a stop in progress is excluded, since it unblocks when that stop finishes.
fn stuck_set(
    order: &[usize],
    signaled: &HashSet<usize>,
    blockers: &HashMap<usize, HashSet<usize>>,
    finished: &HashSet<usize>,
) -> HashSet<usize> {
    let mut stuck: HashSet<usize> = order
        .iter()
        .copied()
        .filter(|idx| !signaled.contains(idx))
        .collect();
    loop {
        let next: HashSet<usize> = stuck
            .iter()
            .copied()
            .filter(|idx| {
                blockers[idx]
                    .iter()
                    .all(|b| finished.contains(b) || stuck.contains(b))
            })
            .collect();
        if next.len() == stuck.len() {
            return stuck;
        }
        stuck = next;
    }
}

/// Follows dependents within `stuck` from `start` until a process repeats, and returns the
/// edge `(from, to)` that closes the walk (so it lies on a cycle). `None` if the walk dead-ends.
fn cycle_edge(
    start: usize,
    order: &[usize],
    blockers: &HashMap<usize, HashSet<usize>>,
    stuck: &HashSet<usize>,
) -> Option<(usize, usize)> {
    let mut seen = HashSet::from([start]);
    let mut current = start;
    loop {
        let next = order
            .iter()
            .copied()
            .find(|dep| stuck.contains(dep) && blockers[&current].contains(dep))?;
        if !seen.insert(next) {
            return Some((current, next));
        }
        current = next;
    }
}

#[cfg(test)]
pub async fn shutdown_all(processes: &mut [ManagedProcess]) {
    let order: Vec<usize> = (0..processes.len()).collect();
    shutdown_ordered(processes, &order, &[]).await;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::state::ProcessState;
    use crate::test_helpers;
    use tokio::time::Duration;

    fn sleep_config() -> crate::config::ProcessConfig {
        test_helpers::sleep_test_config(test_helpers::TEST_SLEEP_SECS)
    }

    #[tokio::test]
    async fn test_shutdown_all_graceful() {
        let cfg1 = sleep_config();
        let cfg2 = sleep_config();

        let mut p1 = ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), cfg1);
        let mut p2 = ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), cfg2);
        p1.spawn(test_exit_channel().0).unwrap();
        p2.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![p1, p2];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
        assert!(!procs[0].is_running());
        assert!(!procs[1].is_running());
    }

    #[tokio::test]
    async fn test_shutdown_all_empty() {
        let mut procs: Vec<ManagedProcess> = vec![];
        shutdown_all(&mut procs).await;
    }

    #[tokio::test]
    async fn test_shutdown_graceful_stop_signal() {
        use std::time::Instant;

        let mut proc = ManagedProcess::new_config(
            "graceful".into(),
            test_helpers::test_uuid(),
            test_helpers::graceful_stop_test_config(),
        );
        let (tx, exit_rx) = test_exit_channel();
        #[cfg(windows)]
        let mut exit_rx = exit_rx;
        proc.spawn(tx).unwrap();
        assert!(proc.is_running());

        // Only the detached helper attaches to the workload console. Caller
        // membership and all standard slots must remain exactly unchanged.
        #[cfg(windows)]
        let console_before = crate::platform::caller_console_state();

        let started = Instant::now();
        proc.request_stop();
        proc.wait_for_stop().await;

        #[cfg(windows)]
        assert_eq!(
            crate::platform::caller_console_state(),
            console_before,
            "graceful stop must leave the caller console as it found it"
        );

        assert_eq!(proc.state(), ProcessState::Stopped);
        #[cfg(windows)]
        {
            let status = exit_rx.recv().await.expect("graceful workload exit").status;
            // A CTRL_BREAK beating the workload's handler ends it through the default handler.
            assert!(
                status.success()
                    || status.code() == Some(windows_sys::Win32::Foundation::STATUS_CONTROL_C_EXIT),
                "CTRL_BREAK should be delivered, not escalated to job force-kill: {status:?}"
            );
        }
        #[cfg(not(windows))]
        let _ = exit_rx;
        assert!(
            started.elapsed().as_secs() < 2,
            "graceful stop should not wait for stop_timeout force-kill (took {:?})",
            started.elapsed()
        );
    }

    #[tokio::test]
    async fn test_shutdown_all_sigkill_on_timeout() {
        // Use ping (via sleep_config) instead of powershell: ping ignores graceful
        // stop on Windows, and powershell.exe is not always on PATH in CI containers.
        let mut proc = ManagedProcess::new_config(
            "stubborn".into(),
            test_helpers::test_uuid(),
            sleep_config(),
        );
        proc.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
    }

    /// Two SIGTERM-ignoring children share one grace period, so shutdown takes about one grace.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_stubborn_children_share_one_grace() {
        use std::time::Instant;

        let dir = tempfile::tempdir().unwrap();
        let mut procs = Vec::new();
        for name in ["stubborn-a", "stubborn-b"] {
            let ready = dir.path().join(format!("{name}.ready"));
            let (cmd, args) = test_helpers::trap_term_sleep_ready(&ready);
            let mut config = test_helpers::make_config(cmd, args);
            config.stop_timeout = Some(2);
            let mut proc =
                ManagedProcess::new_config(name.into(), test_helpers::test_uuid(), config);
            proc.spawn(test_exit_channel().0).unwrap();
            test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;
            procs.push(proc);
        }

        let started = Instant::now();
        shutdown_all(&mut procs).await;
        let elapsed = started.elapsed();

        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
        assert!(
            elapsed < Duration::from_millis(3500),
            "two 2 s stops ran one after another: shutdown took {elapsed:?}"
        );
        assert!(
            elapsed >= Duration::from_millis(1900),
            "shutdown ended before the 2 s grace ran out: took {elapsed:?}"
        );
    }

    /// A short-timeout child is force-killed on its own deadline, not the longer child's.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_short_timeout_killed_on_its_own_deadline() {
        use std::os::unix::process::ExitStatusExt;
        use std::time::Instant;

        let dir = tempfile::tempdir().unwrap();
        let mut procs = Vec::new();
        let mut exits = Vec::new();
        // The long-timeout child is first, so a sequential wait would block on it.
        for (name, stop_timeout) in [("slow-stop", 3), ("fast-stop", 1)] {
            let ready = dir.path().join(format!("{name}.ready"));
            let (cmd, args) = test_helpers::trap_term_sleep_ready(&ready);
            let mut config = test_helpers::make_config(cmd, args);
            config.stop_timeout = Some(stop_timeout);
            let mut proc =
                ManagedProcess::new_config(name.into(), test_helpers::test_uuid(), config);
            let (tx, rx) = test_exit_channel();
            proc.spawn(tx).unwrap();
            test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;
            procs.push(proc);
            exits.push(rx);
        }

        let exit_tasks: Vec<_> = exits
            .into_iter()
            .map(|mut rx| {
                tokio::spawn(async move {
                    let event = rx.recv().await.expect("exit event");
                    (Instant::now(), event.status)
                })
            })
            .collect();

        let started = Instant::now();
        shutdown_all(&mut procs).await;

        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
        let mut results = Vec::new();
        for task in exit_tasks {
            let (at, status) = task.await.unwrap();
            results.push((at.saturating_duration_since(started), status));
        }
        let (slow_exit, slow_status) = &results[0];
        let (fast_exit, fast_status) = &results[1];

        assert!(
            *fast_exit >= Duration::from_millis(1000) && *fast_exit < Duration::from_millis(2000),
            "fast-stop should be force-killed at its 1 s deadline: exited at {fast_exit:?}"
        );
        assert!(
            *slow_exit >= Duration::from_millis(2500) && *slow_exit < Duration::from_millis(4500),
            "slow-stop should be force-killed at its 3 s deadline: exited at {slow_exit:?}"
        );
        for status in [slow_status, fast_status] {
            assert_eq!(
                status.signal(),
                Some(nix::sys::signal::Signal::SIGKILL as i32),
                "child should have been killed by SIGKILL: {status:?}"
            );
        }
    }

    #[tokio::test]
    async fn test_shutdown_all_after_spawn_watcher_owns_handle() {
        let mut proc =
            ManagedProcess::new_config("t".into(), test_helpers::test_uuid(), sleep_config());
        proc.spawn(test_exit_channel().0).unwrap();
        assert!(proc.is_running(), "state should still be Running");
        let mut procs = vec![proc];
        shutdown_all(&mut procs).await;
        assert_eq!(
            procs[0].state(),
            ProcessState::Stopped,
            "shutdown should transition to Stopped via exit watcher"
        );
    }

    #[tokio::test]
    async fn test_shutdown_ordered_explicit_order() {
        let mut p1 =
            ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), sleep_config());
        let mut p2 =
            ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), sleep_config());
        let mut p3 =
            ManagedProcess::new_config("p3".into(), test_helpers::test_uuid(), sleep_config());
        p1.spawn(test_exit_channel().0).unwrap();
        p2.spawn(test_exit_channel().0).unwrap();
        p3.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![p1, p2, p3];
        shutdown_ordered(&mut procs, &[2, 1, 0], &[]).await;

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
        assert_eq!(procs[2].state(), ProcessState::Stopped);
    }

    /// The dependency's marker appears only if it was signaled after the slow dependent finished.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_ordered_signals_dependency_after_dependents() {
        let dir = tempfile::tempdir().unwrap();
        let done = dir.path().join("dependent.done").display().to_string();
        let ordered = dir.path().join("dependency.ordered");
        let dependency_ready = dir.path().join("dependency.ready");
        let dependent_ready = dir.path().join("dependent.ready");

        let dependency_script = format!(
            "trap 'if [ -e \"{done}\" ]; then touch \"{}\"; fi; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            ordered.display(),
            dependency_ready.display()
        );
        let dependent_script = format!(
            "trap 'sleep 0.3; touch \"{done}\"; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            dependent_ready.display()
        );
        let mut dependency = ManagedProcess::new_config(
            "dependency".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config("/bin/sh", vec!["-c".into(), dependency_script]),
        );
        let mut dependent = ManagedProcess::new_config(
            "dependent".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config("/bin/sh", vec!["-c".into(), dependent_script]),
        );
        dependency.spawn(test_exit_channel().0).unwrap();
        dependent.spawn(test_exit_channel().0).unwrap();
        test_helpers::wait_for_file(&dependency_ready, Duration::from_secs(10)).await;
        test_helpers::wait_for_file(&dependent_ready, Duration::from_secs(10)).await;

        let mut procs = vec![dependency, dependent];
        shutdown_ordered(&mut procs, &[1, 0], &[vec![1], vec![]]).await;

        assert!(
            std::path::Path::new(&done).exists(),
            "dependent should have finished stopping"
        );
        assert!(
            ordered.exists(),
            "dependency was signaled before its dependent finished stopping"
        );
        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
    }

    /// A cycle must not wait for an unrelated stop in progress. `c` ignores TERM with a
    /// 5 s grace. `a` and `b` form a cycle and must exit well before `c` is killed.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_ordered_cycle_not_delayed_by_unrelated_stop() {
        use std::time::Instant;

        let dir = tempfile::tempdir().unwrap();
        let ready = dir.path().join("c.ready");
        let (cmd, args) = test_helpers::trap_term_sleep_ready(&ready);
        let mut stubborn_config = test_helpers::make_config(cmd, args);
        stubborn_config.stop_timeout = Some(5);
        let mut c =
            ManagedProcess::new_config("c".into(), test_helpers::test_uuid(), stubborn_config);
        let mut a =
            ManagedProcess::new_config("a".into(), test_helpers::test_uuid(), sleep_config());
        let mut b =
            ManagedProcess::new_config("b".into(), test_helpers::test_uuid(), sleep_config());
        c.spawn(test_exit_channel().0).unwrap();
        let (a_tx, mut a_rx) = test_exit_channel();
        let (b_tx, mut b_rx) = test_exit_channel();
        a.spawn(a_tx).unwrap();
        b.spawn(b_tx).unwrap();
        test_helpers::wait_for_file(&ready, Duration::from_secs(10)).await;

        let started = Instant::now();
        let a_exit = tokio::spawn(async move {
            a_rx.recv().await.expect("a exit");
            Instant::now().saturating_duration_since(started)
        });
        let b_exit = tokio::spawn(async move {
            b_rx.recv().await.expect("b exit");
            Instant::now().saturating_duration_since(started)
        });

        let mut procs = vec![c, a, b];
        // dependents[1] == [2] and dependents[2] == [1]: a and b form a cycle.
        let dependents = vec![vec![], vec![2], vec![1]];
        shutdown_ordered(&mut procs, &[0, 1, 2], &dependents).await;

        let a_at = a_exit.await.unwrap();
        let b_at = b_exit.await.unwrap();
        assert!(
            a_at < Duration::from_secs(2) && b_at < Duration::from_secs(2),
            "cycle waited for the unrelated stop: a exited at {a_at:?}, b at {b_at:?}"
        );
        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
    }

    /// Shutdown must finish despite a dependency cycle.
    #[tokio::test]
    async fn test_shutdown_ordered_breaks_dependency_cycle() {
        let mut p1 =
            ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), sleep_config());
        let mut p2 =
            ManagedProcess::new_config("p2".into(), test_helpers::test_uuid(), sleep_config());
        p1.spawn(test_exit_channel().0).unwrap();
        p2.spawn(test_exit_channel().0).unwrap();

        let mut procs = vec![p1, p2];
        let completed = tokio::time::timeout(
            Duration::from_secs(60),
            shutdown_ordered(&mut procs, &[0, 1], &[vec![1], vec![0]]),
        )
        .await;
        assert!(completed.is_ok(), "shutdown hung on a dependency cycle");

        assert_eq!(procs[0].state(), ProcessState::Stopped);
        assert_eq!(procs[1].state(), ProcessState::Stopped);
    }

    /// `a` and `b` form a cycle, and `a` is a dependent of `db`. Breaking the cycle must keep
    /// `db` waiting for `a`.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_ordered_breaks_cycle_not_upstream_dependency() {
        let dir = tempfile::tempdir().unwrap();
        let a_done = dir.path().join("a.done").display().to_string();
        let db_ordered = dir.path().join("db.ordered");
        let db_ready = dir.path().join("db.ready");
        let a_ready = dir.path().join("a.ready");

        let db_script = format!(
            "trap 'if [ -e \"{a_done}\" ]; then touch \"{}\"; fi; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            db_ordered.display(),
            db_ready.display()
        );
        let a_script = format!(
            "trap 'touch \"{a_done}\"; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            a_ready.display()
        );
        let mut db = ManagedProcess::new_config(
            "db".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config("/bin/sh", vec!["-c".into(), db_script]),
        );
        let mut a = ManagedProcess::new_config(
            "a".into(),
            test_helpers::test_uuid(),
            test_helpers::make_config("/bin/sh", vec!["-c".into(), a_script]),
        );
        let mut b =
            ManagedProcess::new_config("b".into(), test_helpers::test_uuid(), sleep_config());
        db.spawn(test_exit_channel().0).unwrap();
        a.spawn(test_exit_channel().0).unwrap();
        b.spawn(test_exit_channel().0).unwrap();
        test_helpers::wait_for_file(&db_ready, Duration::from_secs(10)).await;
        test_helpers::wait_for_file(&a_ready, Duration::from_secs(10)).await;

        let mut procs = vec![db, a, b];
        let dependents = vec![vec![1], vec![2], vec![1]];
        let completed = tokio::time::timeout(
            Duration::from_secs(60),
            shutdown_ordered(&mut procs, &[0, 1, 2], &dependents),
        )
        .await;
        assert!(completed.is_ok(), "shutdown hung on a dependency cycle");

        assert!(
            db_ordered.exists(),
            "db was signaled before its dependent a had finished stopping"
        );
        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
    }

    /// Cycles 0 <-> 1 and 2 <-> 3 are joined by the edge "2 is a dependent of 0". Breaking the
    /// cycles must keep 0 waiting for 2.
    #[cfg(unix)]
    #[tokio::test]
    async fn test_shutdown_ordered_breaks_two_connected_cycles() {
        let dir = tempfile::tempdir().unwrap();
        let p2_done = dir.path().join("p2.done").display().to_string();
        let p0_ordered = dir.path().join("p0.ordered");
        let p0_ready = dir.path().join("p0.ready");
        let p2_ready = dir.path().join("p2.ready");

        let p0_script = format!(
            "trap 'if [ -e \"{p2_done}\" ]; then touch \"{}\"; fi; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            p0_ordered.display(),
            p0_ready.display()
        );
        let p2_script = format!(
            "trap 'touch \"{p2_done}\"; exit 0' TERM; touch \"{}\"; while :; do sleep 0.05; done",
            p2_ready.display()
        );
        let mut procs = vec![
            ManagedProcess::new_config(
                "p0".into(),
                test_helpers::test_uuid(),
                test_helpers::make_config("/bin/sh", vec!["-c".into(), p0_script]),
            ),
            ManagedProcess::new_config("p1".into(), test_helpers::test_uuid(), sleep_config()),
            ManagedProcess::new_config(
                "p2".into(),
                test_helpers::test_uuid(),
                test_helpers::make_config("/bin/sh", vec!["-c".into(), p2_script]),
            ),
            ManagedProcess::new_config("p3".into(), test_helpers::test_uuid(), sleep_config()),
        ];
        for proc in &mut procs {
            proc.spawn(test_exit_channel().0).unwrap();
        }
        test_helpers::wait_for_file(&p0_ready, Duration::from_secs(10)).await;
        test_helpers::wait_for_file(&p2_ready, Duration::from_secs(10)).await;

        let dependents = vec![vec![1, 2], vec![0], vec![3], vec![2]];
        let completed = tokio::time::timeout(
            Duration::from_secs(60),
            shutdown_ordered(&mut procs, &[0, 1, 2, 3], &dependents),
        )
        .await;
        assert!(completed.is_ok(), "shutdown hung on two connected cycles");

        assert!(
            p0_ordered.exists(),
            "p0 was signaled before its dependent p2 had finished stopping"
        );
        for proc in &procs {
            assert_eq!(proc.state(), ProcessState::Stopped);
        }
    }

    #[tokio::test]
    async fn test_shutdown_ordered_empty() {
        let mut procs: Vec<ManagedProcess> = vec![];
        shutdown_ordered(&mut procs, &[], &[]).await;
    }
}
