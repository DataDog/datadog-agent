// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Run a `secret_backend_command` synchronously and capture stdout.
//!
//! Platform code chooses the process identity (see `platform::{unix,windows}/secret_backend`).

use std::io::{Read, Write};
use std::process::{Child, Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};

use anyhow::{Context, Result, bail};

pub(crate) struct BackendRun<'a> {
    pub command: &'a str,
    pub arguments: &'a [String],
    pub payload: &'a str,
    pub timeout: Duration,
    pub max_output_bytes: usize,
}

/// Spawn with the inherited supervisor token (`std::process::Command`).
pub(crate) fn exec_inherited_token(run: &BackendRun<'_>) -> Result<String> {
    spawn_and_capture(run, |_| Ok(()))
}

/// Spawn after optional `Command` setup (e.g. Unix `pre_exec` to drop to the agent user).
pub(crate) fn spawn_and_capture(
    run: &BackendRun<'_>,
    configure: impl FnOnce(&mut Command) -> Result<()>,
) -> Result<String> {
    let mut child = Command::new(run.command);
    child
        .args(run.arguments)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    configure(&mut child).with_context(|| format!("configure secret backend {}", run.command))?;
    let mut child = child
        .spawn()
        .with_context(|| format!("spawn secret backend {}", run.command))?;

    if let Some(mut stdin) = child.stdin.take() {
        stdin
            .write_all(run.payload.as_bytes())
            .context("write secret backend payload")?;
    }

    let stdout = child
        .stdout
        .take()
        .context("read secret backend stdout")?;
    let pid = child.id();
    let deadline = Instant::now() + run.timeout;
    let command = run.command;
    let timeout_secs = run.timeout.as_secs();

    wait_with_stdout_drain(
        stdout,
        run.max_output_bytes,
        move || kill_process_by_pid(pid),
        move || wait_for_command_child(&mut child, deadline, command, timeout_secs),
    )
}

/// Read stdout on a background thread while waiting for the child to finish.
///
/// Secret backends may emit more than the OS pipe buffer holds before exiting; draining
/// concurrently avoids a deadlock where the child blocks on a full pipe and we time out.
/// When output exceeds `max_output_bytes`, `kill_child` runs immediately so the backend
/// cannot wedge on a full pipe until the wait loop times out.
pub(crate) fn wait_with_stdout_drain<R, K, W>(
    stdout: R,
    max_output_bytes: usize,
    kill_child: K,
    wait_for_child: W,
) -> Result<String>
where
    R: Read + Send + 'static,
    K: Fn() + Send + 'static,
    W: FnOnce() -> Result<()>,
{
    thread::scope(|scope| {
        let reader = scope.spawn(move || read_stdout_or_kill(stdout, max_output_bytes, kill_child));

        let wait_result = wait_for_child();
        let output = reader
            .join()
            .map_err(|_| anyhow::anyhow!("secret backend stdout reader panicked"))??;
        wait_result?;
        Ok(output)
    })
}

fn read_stdout_or_kill<R, K>(stdout: R, max_output_bytes: usize, kill_child: K) -> Result<String>
where
    R: Read,
    K: Fn(),
{
    match read_limited_stdout(Some(stdout), max_output_bytes) {
        Ok(output) => Ok(output),
        Err(ReadStdoutError::LimitExceeded(limit)) => {
            kill_child();
            bail!("secret backend output exceeded {limit} bytes");
        }
        Err(ReadStdoutError::Other(err)) => Err(err),
    }
}

fn wait_for_command_child(
    child: &mut Child,
    deadline: Instant,
    command: &str,
    timeout_secs: u64,
) -> Result<()> {
    let status = loop {
        match child.try_wait().context("poll secret backend")? {
            Some(status) => break status,
            None if Instant::now() >= deadline => {
                let _ = child.kill();
                let _ = child.wait();
                bail!("secret backend {command} timed out after {timeout_secs} seconds");
            }
            None => thread::sleep(Duration::from_millis(50)),
        }
    };
    if !status.success() {
        bail!("secret backend {command} exited with {status}");
    }
    Ok(())
}

pub(crate) enum ReadStdoutError {
    LimitExceeded(usize),
    Other(anyhow::Error),
}

pub(crate) fn read_limited_stdout(
    stdout: Option<impl Read>,
    max_output_bytes: usize,
) -> std::result::Result<String, ReadStdoutError> {
    let stdout = stdout
        .context("read secret backend stdout")
        .map_err(ReadStdoutError::Other)?;
    let mut output = Vec::new();
    stdout
        .take(max_output_bytes as u64 + 1)
        .read_to_end(&mut output)
        .context("read secret backend stdout")
        .map_err(ReadStdoutError::Other)?;
    if output.len() > max_output_bytes {
        return Err(ReadStdoutError::LimitExceeded(max_output_bytes));
    }
    String::from_utf8(output)
        .context("decode secret backend stdout as UTF-8")
        .map_err(ReadStdoutError::Other)
}

#[cfg(unix)]
fn kill_process_by_pid(pid: u32) {
    use nix::sys::signal::{Signal, kill};
    use nix::unistd::Pid;
    let _ = kill(Pid::from_raw(pid as i32), Signal::SIGKILL);
}

#[cfg(windows)]
fn kill_process_by_pid(pid: u32) {
    use windows_sys::Win32::Foundation::CloseHandle;
    use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_TERMINATE, TerminateProcess};

    unsafe {
        let handle = OpenProcess(PROCESS_TERMINATE, 0, pid);
        if handle.is_null() {
            return;
        }
        TerminateProcess(handle, 1);
        CloseHandle(handle);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn wait_with_stdout_drain_reads_concurrently() {
        let (reader, mut writer) = std::io::pipe().expect("pipe");
        let payload = vec![b'a'; 131_072];
        let payload_len = payload.len();
        let writer = thread::spawn(move || {
            writer.write_all(&payload).expect("write payload");
        });

        let output = wait_with_stdout_drain(
            reader,
            1024 * 1024,
            || {},
            || {
                writer.join().expect("writer thread");
                Ok(())
            },
        )
        .expect("concurrent drain");

        assert_eq!(output.len(), payload_len);
    }

    #[test]
    fn wait_with_stdout_drain_errors_when_output_exceeds_limit() {
        let (reader, mut writer) = std::io::pipe().expect("pipe");
        let writer = thread::spawn(move || {
            writer
                .write_all(&vec![b'a'; 1025])
                .expect("write payload");
        });

        let err = wait_with_stdout_drain(reader, 1024, || {}, || {
            writer.join().expect("writer thread");
            Ok(())
        })
        .unwrap_err();

        assert!(
            err.to_string().contains("output exceeded 1024 bytes"),
            "unexpected error: {err:#}"
        );
    }

    #[cfg(unix)]
    #[test]
    fn spawn_and_capture_drains_stdout_while_child_runs() {
        // Pipe buffers are typically 64 KiB; emit more before exit to catch wait-then-read deadlocks.
        let run = BackendRun {
            command: "sh",
            arguments: &[
                "-c".into(),
                "perl -e 'print \"a\" x 131072'".into(),
            ],
            payload: "secret-handle",
            timeout: Duration::from_secs(5),
            max_output_bytes: 1024 * 1024,
        };
        let output = spawn_and_capture(&run, |_| Ok(())).expect("stdout drain");
        assert_eq!(output.len(), 131_072);
        assert!(output.chars().all(|c| c == 'a'));
    }

    #[cfg(unix)]
    #[test]
    fn spawn_and_capture_kills_child_when_output_exceeds_limit() {
        let run = BackendRun {
            command: "sh",
            arguments: &[
                "-c".into(),
                "perl -e 'print \"a\" x 131072'".into(),
            ],
            payload: "secret-handle",
            timeout: Duration::from_secs(5),
            max_output_bytes: 1024,
        };
        let err = spawn_and_capture(&run, |_| Ok(())).unwrap_err();
        assert!(
            err.to_string().contains("output exceeded 1024 bytes"),
            "unexpected error: {err:#}"
        );
    }
}
