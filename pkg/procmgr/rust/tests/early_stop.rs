// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! A SIGTERM delivered after the early stop handler is installed and before the Tokio runtime
//! exists must end the run before the start pass begins.
//!
//! This binary holds one test because the stop flag is process-wide and never cleared.

#![cfg(unix)]

use std::sync::{Arc, Mutex};
use std::time::Duration;

use dd_procmgrd::config::YamlConfigLoader;
use dd_procmgrd::manager::ProcessManager;
use dd_procmgrd::platform;
use dd_procmgrd::uuid_gen::V4UuidGenerator;
use nix::sys::signal::{Signal, raise};

static LOGS: Mutex<Vec<String>> = Mutex::new(Vec::new());

struct CaptureLogger;

impl log::Log for CaptureLogger {
    fn enabled(&self, _: &log::Metadata) -> bool {
        true
    }

    fn log(&self, record: &log::Record) {
        LOGS.lock().unwrap().push(record.args().to_string());
    }

    fn flush(&self) {}
}

#[test]
fn test_stop_before_runtime_ends_the_run_before_startup() {
    log::set_logger(&CaptureLogger).expect("install logger");
    log::set_max_level(log::LevelFilter::Info);

    let dir = tempfile::tempdir().expect("temp dir");
    // A critical process that cannot spawn. Any spawn attempt fails and gives exit code 1.
    std::fs::write(
        dir.path().join("critical.yaml"),
        "command: /nonexistent/dd-procmgr-missing-binary\nauto_start: true\nexit_with_supervisor: true\n",
    )
    .expect("write config");
    // SAFETY: the only other thread is libtest's main thread, which is parked waiting for this
    // test, and nothing else reads the environment.
    unsafe {
        std::env::set_var("DD_PM_CONFIG_DIR", dir.path());
        std::env::set_var("DD_PM_SOCKET_PATH", dir.path().join("procmgrd.sock"));
    }

    platform::install_early_stop_handler();
    // The handler runs on this thread before `raise` returns, so the stop is recorded before the
    // runtime below is built.
    raise(Signal::SIGTERM).expect("raise SIGTERM");

    let runtime = tokio::runtime::Runtime::new().expect("runtime");
    let exit_code = runtime.block_on(async {
        let manager = ProcessManager::new(
            Arc::new(YamlConfigLoader::from_env()),
            Arc::new(V4UuidGenerator),
        );
        // `pending()` never resolves, so only the recorded stop can end the run. Without it the
        // manager would try to start "critical" and then wait forever.
        let exit_code = tokio::time::timeout(
            Duration::from_secs(10),
            manager.run_with_shutdown(std::future::pending()),
        )
        .await
        .expect("the recorded stop did not end the run")
        .expect("run_with_shutdown failed");

        tokio::time::timeout(Duration::from_secs(10), platform::shutdown_signal())
            .await
            .expect("the recorded stop did not resolve shutdown_signal");
        exit_code
    });

    assert_eq!(
        exit_code, 0,
        "a stop before startup should exit cleanly, a spawn attempt would give exit code 1"
    );
    let logs = LOGS.lock().unwrap().clone();
    assert!(
        logs.iter()
            .any(|l| l.contains("stop requested before startup")),
        "the run did not end before the start pass\n{}",
        logs.join("\n")
    );
    assert!(
        !logs.iter().any(|l| l.contains("[critical]")),
        "the critical process was touched after a stop before startup\n{}",
        logs.join("\n")
    );
    assert_eq!(
        logs.iter()
            .filter(|l| l.contains("received SIGTERM"))
            .count(),
        1,
        "the stop should be logged once\n{}",
        logs.join("\n")
    );
}
