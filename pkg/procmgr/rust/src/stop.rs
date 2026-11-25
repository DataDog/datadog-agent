// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Filesystem calls that a stop request must not wait behind.
//!
//! The start pass and the catalog load are synchronous, so the shutdown future cannot be polled
//! while a filesystem call blocks. Those calls run on a helper thread that the caller bounds
//! with a deadline and the stop flag.
//!
//! Limits: a caller waits up to [`CALL_DEADLINE`] per distinct stalled path, on a runtime worker
//! (with the process table locked in the start pass). On Unix, `Command::spawn` waits on the exec
//! pipe, so a `command` or `working_dir` on a hung mount can still block a spawn. Refusals are
//! keyed by the literal path, so a symlink to the same file is a different path.

use std::io;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Mutex, MutexGuard, PoisonError, mpsc};
use std::thread;
use std::time::{Duration, Instant};

use log::{debug, error};

/// Set by the early SIGTERM and SIGINT handler, which tokio's handler chains to (on Unix), and by
/// the Windows service stop and console control handlers. Never cleared, so it outlives any one
/// `ProcessManager` run.
static STOP: AtomicBool = AtomicBool::new(false);

/// The stop signal number that set `STOP`, or 0. Stored before `STOP` is set.
#[cfg(unix)]
static STOP_SIGNAL: std::sync::atomic::AtomicI32 = std::sync::atomic::AtomicI32::new(0);

/// Bound for one filesystem call. Reloads and creates run in the control loop, so an unbounded
/// call there would also block gRPC.
#[cfg(not(test))]
pub(crate) const CALL_DEADLINE: Duration = Duration::from_secs(30);
/// Shorter in unit tests, where a stalled call is exercised directly.
#[cfg(test)]
pub(crate) const CALL_DEADLINE: Duration = Duration::from_secs(2);

/// How long a path stays refused after its reader was abandoned.
const RETRY_AFTER: Duration = Duration::from_secs(60);

/// Most blocked readers for one path. A retry after [`RETRY_AFTER`] reads a replaced file, since
/// the first reader stays blocked on the old inode.
const MAX_BLOCKED_PER_PATH: usize = 2;

/// Most blocked readers in total, to bound the thread count.
const MAX_BLOCKED_TOTAL: usize = 32;

/// How often a waiting caller checks the stop flag and the deadline.
const POLL_INTERVAL: Duration = Duration::from_millis(20);

/// A reader its caller gave up on that is still blocked. `ticket` keeps [`Release`] from
/// removing another call's entry for the same path.
struct Abandoned {
    path: PathBuf,
    ticket: u64,
    since: Instant,
}

static ABANDONED: Mutex<Vec<Abandoned>> = Mutex::new(Vec::new());
static NEXT_TICKET: AtomicU64 = AtomicU64::new(0);

/// Removes the call's entry when its helper thread ends, including a panic in the call.
struct Release(u64);

impl Drop for Release {
    fn drop(&mut self) {
        lock_abandoned().retain(|a| a.ticket != self.0);
    }
}

/// Records a stop request from `signal`. Only lock-free atomic operations, so it is safe inside a
/// signal handler.
#[cfg(unix)]
pub(crate) fn request_stop(signal: i32) {
    // Keep the first signal, so the name matches the one that set the flag.
    let _ = STOP_SIGNAL.compare_exchange(0, signal, Ordering::SeqCst, Ordering::SeqCst);
    STOP.store(true, Ordering::SeqCst);
}

/// Records a stop request. Only an atomic store, so it is safe inside a console control handler.
#[cfg(not(unix))]
pub(crate) fn request_stop() {
    STOP.store(true, Ordering::SeqCst);
}

/// Name of the recorded stop signal. Call it after `stop_requested_flag` returns true.
#[cfg(unix)]
pub(crate) fn stop_signal_name() -> &'static str {
    use nix::libc::{SIGINT, SIGTERM};
    match STOP_SIGNAL.load(Ordering::SeqCst) {
        SIGTERM => "SIGTERM",
        SIGINT => "SIGINT",
        _ => "stop signal",
    }
}

/// Name of the stop signal on platforms without signal numbers.
#[cfg(not(unix))]
pub(crate) fn stop_signal_name() -> &'static str {
    "stop signal"
}

static LOGGED: std::sync::Once = std::sync::Once::new();

/// Logs `received <signal>` once per process. Both the main thread and the watcher may see the
/// stop. A second caller waits until the first has finished logging.
pub(crate) fn log_received(signal: &str) {
    LOGGED.call_once(|| log::info!("received {signal}"));
}

pub(crate) fn stop_requested_flag() -> bool {
    STOP.load(Ordering::SeqCst)
}

/// Reads a config or environment file without outliving a stop request or [`CALL_DEADLINE`].
pub(crate) fn read_config_to_string(path: &Path) -> io::Result<String> {
    let owned = path.to_path_buf();
    bounded(path, move || std::fs::read_to_string(owned)).and_then(|result| result)
}

/// Runs a filesystem call for `path` on a helper thread, for at most [`CALL_DEADLINE`] or until
/// a stop. A call that gives up leaves its thread blocked and `path` refused for a while. A
/// refusal, timeout or stop returns an error that callers must treat as a hold, not as a missing
/// file. The error does not name the path, so callers add it. A result that is ready when a
/// stop is seen is dropped (`Interrupted`), so a stop racing the last call of a spawn starts
/// no child.
pub(crate) fn bounded<T: Send + 'static>(
    path: &Path,
    op: impl FnOnce() -> T + Send + 'static,
) -> io::Result<T> {
    bounded_with(path, CALL_DEADLINE, &STOP, op)
}

fn lock_abandoned() -> MutexGuard<'static, Vec<Abandoned>> {
    ABANDONED.lock().unwrap_or_else(PoisonError::into_inner)
}

/// Why a new call for `path` must not start, or `None` when it may.
fn refusal(
    abandoned: &[Abandoned],
    path: &Path,
    retry_after: Duration,
    max_total: usize,
) -> Option<&'static str> {
    let mut for_path = 0;
    for entry in abandoned.iter().filter(|a| a.path == path) {
        if entry.since.elapsed() < retry_after {
            return Some("an earlier read is still blocked, not starting another");
        }
        for_path += 1;
    }
    if for_path >= MAX_BLOCKED_PER_PATH {
        return Some("earlier reads of this path are still blocked, not starting another");
    }
    if abandoned.len() >= max_total {
        return Some("too many reads are still blocked, not starting another");
    }
    None
}

fn bounded_with<T: Send + 'static>(
    path: &Path,
    deadline: Duration,
    stop: &AtomicBool,
    op: impl FnOnce() -> T + Send + 'static,
) -> io::Result<T> {
    if stop.load(Ordering::SeqCst) {
        return Err(io::Error::new(
            io::ErrorKind::Interrupted,
            "stop requested, not reading",
        ));
    }
    if let Some(reason) = refusal(&lock_abandoned(), path, RETRY_AFTER, MAX_BLOCKED_TOTAL) {
        return Err(io::Error::new(io::ErrorKind::TimedOut, reason));
    }

    let (tx, rx) = mpsc::channel();
    let ticket = NEXT_TICKET.fetch_add(1, Ordering::SeqCst);
    thread::Builder::new()
        .name("fs-call".to_string())
        .spawn(move || {
            let _release = Release(ticket);
            let result = op();
            // Held across the send: a caller giving up takes the same lock before recording
            // its entry, so it cannot record one for a call that finished.
            let abandoned = lock_abandoned();
            let _ = tx.send(result);
            drop(abandoned);
        })?;

    let started = Instant::now();
    loop {
        match rx.recv_timeout(POLL_INTERVAL) {
            Ok(result) => return stopped_or(result, stop),
            Err(mpsc::RecvTimeoutError::Disconnected) => return Err(reader_ended()),
            Err(mpsc::RecvTimeoutError::Timeout) => {
                let stopping = stop.load(Ordering::SeqCst);
                if !stopping && started.elapsed() < deadline {
                    continue;
                }
                let mut abandoned = lock_abandoned();
                // A result sent since the last poll wins, and a dead reader is not recorded.
                match rx.try_recv() {
                    Ok(result) => return stopped_or(result, stop),
                    Err(mpsc::TryRecvError::Disconnected) => return Err(reader_ended()),
                    Err(mpsc::TryRecvError::Empty) => {}
                }
                abandoned.push(Abandoned {
                    path: path.to_path_buf(),
                    ticket,
                    since: Instant::now(),
                });
                return Err(if stopping {
                    debug!("{}: stop requested while reading", path.display());
                    io::Error::new(io::ErrorKind::Interrupted, "stop requested while reading")
                } else {
                    error!(
                        "{}: no result within {deadline:?}, leaving the read blocked ({} blocked in total)",
                        path.display(),
                        abandoned.len()
                    );
                    io::Error::new(
                        io::ErrorKind::TimedOut,
                        format!("no result within {deadline:?}"),
                    )
                });
            }
        }
    }
}

/// A ready result, unless a stop was requested, which discards it.
fn stopped_or<T>(result: T, stop: &AtomicBool) -> io::Result<T> {
    if stop.load(Ordering::SeqCst) {
        return Err(io::Error::new(
            io::ErrorKind::Interrupted,
            "stop requested, discarding the result",
        ));
    }
    Ok(result)
}

fn reader_ended() -> io::Error {
    io::Error::other("reader thread ended without a result")
}

/// Lets a unit test stall the read of a regular file (the loader never opens a FIFO).
#[cfg(test)]
pub(crate) mod testing {
    use super::*;

    static STALLED: Mutex<Vec<PathBuf>> = Mutex::new(Vec::new());

    /// Makes [`park_if_stalled`] block for `path` from now on.
    pub(crate) fn stall(path: &Path) {
        STALLED
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push(path.to_path_buf());
    }

    /// Parks the calling thread for good when `path` was passed to [`stall`].
    pub(crate) fn park_if_stalled(path: &Path) {
        let stalled = STALLED
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .iter()
            .any(|p| p == path);
        if stalled {
            loop {
                thread::park();
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    /// A call that signals `started`, then blocks until `release` is sent to or dropped.
    fn blocked_call(
        started: mpsc::Sender<()>,
        release: mpsc::Receiver<()>,
    ) -> impl FnOnce() -> u32 + Send + 'static {
        move || {
            let _ = started.send(());
            let _ = release.recv();
            7
        }
    }

    /// Ages the entries of `path` by `by`, as the passing of that time would.
    fn age(path: &Path, by: Duration) {
        for entry in lock_abandoned().iter_mut().filter(|a| a.path == path) {
            entry.since = entry.since.checked_sub(by).unwrap();
        }
    }

    fn blocked_entries(path: &Path) -> usize {
        lock_abandoned().iter().filter(|a| a.path == path).count()
    }

    /// Polls, bounded, for a condition that a helper thread brings about.
    fn wait_until(what: &str, condition: impl Fn() -> bool) {
        let deadline = Instant::now() + Duration::from_secs(10);
        while !condition() {
            assert!(Instant::now() < deadline, "{what}");
            thread::sleep(Duration::from_millis(5));
        }
    }

    fn scratch_path(name: &str) -> PathBuf {
        std::env::temp_dir().join(format!("procmgr-stop-{name}-{}", std::process::id()))
    }

    #[test]
    fn reads_a_regular_file() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("cfg.yaml");
        std::fs::write(&file, "command: /bin/true\n").unwrap();
        let stop = AtomicBool::new(false);
        let owned = file.clone();
        let contents = bounded_with(&file, Duration::from_secs(5), &stop, move || {
            std::fs::read_to_string(owned)
        })
        .and_then(|result| result)
        .unwrap();
        assert_eq!(contents, "command: /bin/true\n");
    }

    #[test]
    fn a_stop_already_requested_starts_no_call() {
        let stop = AtomicBool::new(true);
        let ran = Arc::new(AtomicBool::new(false));
        let ran_in_call = Arc::clone(&ran);
        let err = bounded_with(
            Path::new("/unused"),
            Duration::from_secs(5),
            &stop,
            move || ran_in_call.store(true, Ordering::SeqCst),
        )
        .unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::Interrupted, "{err}");
        assert!(!ran.load(Ordering::SeqCst), "the call must not run");
    }

    #[test]
    fn a_stop_interrupts_a_blocked_call() {
        let path = scratch_path("interrupt");
        let stop = Arc::new(AtomicBool::new(false));
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();
        let raiser = Arc::clone(&stop);
        let raise = thread::spawn(move || {
            started_rx.recv().unwrap();
            raiser.store(true, Ordering::SeqCst);
        });
        let started = Instant::now();
        let err = bounded_with(
            &path,
            Duration::from_secs(30),
            &stop,
            blocked_call(started_tx, release_rx),
        )
        .unwrap_err();
        raise.join().unwrap();
        assert_eq!(err.kind(), io::ErrorKind::Interrupted, "{err}");
        assert!(started.elapsed() < Duration::from_secs(10));
        assert_eq!(blocked_entries(&path), 1, "the blocked call stays recorded");
        release_tx.send(()).unwrap();
        wait_until("the entry was never cleared", || {
            blocked_entries(&path) == 0
        });
    }

    #[test]
    fn a_result_that_is_ready_when_a_stop_is_seen_is_discarded() {
        let stop = Arc::new(AtomicBool::new(false));
        let raised_by_call = Arc::clone(&stop);
        let err = bounded_with(
            Path::new("/unused"),
            Duration::from_secs(30),
            &stop,
            move || raised_by_call.store(true, Ordering::SeqCst),
        )
        .unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::Interrupted, "{err}");
    }

    #[test]
    fn a_call_that_panics_clears_its_entry() {
        let path = scratch_path("panics");
        let stop = AtomicBool::new(false);
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel::<()>();

        let err = bounded_with(&path, Duration::from_millis(100), &stop, move || -> u32 {
            let _ = started_tx.send(());
            let _ = release_rx.recv();
            panic!("the call fails after it was given up");
        })
        .unwrap_err();
        started_rx.recv().unwrap();
        assert_eq!(err.kind(), io::ErrorKind::TimedOut, "{err}");
        assert_eq!(blocked_entries(&path), 1);

        release_tx.send(()).unwrap();
        wait_until("a panicking call left its entry behind", || {
            blocked_entries(&path) == 0
        });
    }

    #[test]
    fn a_path_is_refused_while_blocked_and_read_again_once_the_interval_passed() {
        let path = scratch_path("retry");
        let stop = AtomicBool::new(false);
        let (started_tx, started_rx) = mpsc::channel();
        let (release_tx, release_rx) = mpsc::channel();

        let err = bounded_with(
            &path,
            Duration::from_millis(100),
            &stop,
            blocked_call(started_tx, release_rx),
        )
        .unwrap_err();
        started_rx.recv().unwrap();
        assert_eq!(err.kind(), io::ErrorKind::TimedOut, "{err}");

        let err = bounded_with(&path, Duration::from_secs(30), &stop, || ()).unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::TimedOut, "{err}");
        assert!(err.to_string().contains("still blocked"), "{err}");

        age(&path, RETRY_AFTER + Duration::from_secs(1));
        let value = bounded_with(&path, Duration::from_secs(5), &stop, || 5).unwrap();
        assert_eq!(value, 5);

        release_tx.send(()).unwrap();
        wait_until("the entry was never cleared", || {
            blocked_entries(&path) == 0
        });
    }

    #[test]
    fn a_path_with_two_blocked_readers_is_refused_until_one_returns() {
        let path = scratch_path("cap");
        let stop = AtomicBool::new(false);
        let mut releases = Vec::new();
        for _ in 0..MAX_BLOCKED_PER_PATH {
            let (started_tx, started_rx) = mpsc::channel();
            let (release_tx, release_rx) = mpsc::channel();
            let err = bounded_with(
                &path,
                Duration::from_millis(100),
                &stop,
                blocked_call(started_tx, release_rx),
            )
            .unwrap_err();
            started_rx.recv().unwrap();
            assert_eq!(err.kind(), io::ErrorKind::TimedOut, "{err}");
            releases.push(release_tx);
            age(&path, RETRY_AFTER + Duration::from_secs(1));
        }
        assert_eq!(blocked_entries(&path), MAX_BLOCKED_PER_PATH);

        let err = bounded_with(&path, Duration::from_secs(30), &stop, || ()).unwrap_err();
        assert_eq!(err.kind(), io::ErrorKind::TimedOut, "{err}");
        assert!(err.to_string().contains("this path"), "{err}");

        for release in releases {
            release.send(()).unwrap();
        }
        wait_until("the entries were never cleared", || {
            blocked_entries(&path) == 0
        });
        assert_eq!(
            bounded_with(&path, Duration::from_secs(5), &stop, || 1).unwrap(),
            1,
            "a path is readable again once its readers returned"
        );
    }

    #[test]
    fn too_many_blocked_readers_refuse_any_path() {
        let entry = |name: &str| Abandoned {
            path: PathBuf::from(name),
            ticket: 0,
            since: Instant::now()
                .checked_sub(RETRY_AFTER + Duration::from_secs(1))
                .unwrap(),
        };
        let abandoned: Vec<_> = (0..4).map(|i| entry(&format!("/blocked/{i}"))).collect();
        assert!(refusal(&abandoned, Path::new("/other"), RETRY_AFTER, 5).is_none());
        let reason = refusal(&abandoned, Path::new("/other"), RETRY_AFTER, 4).unwrap();
        assert!(reason.contains("too many"), "{reason}");
    }
}
