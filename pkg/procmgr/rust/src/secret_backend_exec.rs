// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Run a `secret_backend_command` synchronously and capture stdout.
//!
//! Platform code chooses the process identity (see `platform::{unix,windows}/secret_backend`).

use std::io::{Read, Write};
use std::process::{Command, Stdio};
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

    let deadline = Instant::now() + run.timeout;
    let status = loop {
        match child.try_wait().context("poll secret backend")? {
            Some(status) => break status,
            None if Instant::now() >= deadline => {
                let _ = child.kill();
                let _ = child.wait();
                bail!(
                    "secret backend {} timed out after {} seconds",
                    run.command,
                    run.timeout.as_secs()
                );
            }
            None => thread::sleep(Duration::from_millis(50)),
        }
    };
    if !status.success() {
        bail!("secret backend {} exited with {status}", run.command);
    }

    read_limited_stdout(child.stdout.take(), run.max_output_bytes)
}

pub(crate) fn read_limited_stdout(
    stdout: Option<impl Read>,
    max_output_bytes: usize,
) -> Result<String> {
    let stdout = stdout.context("read secret backend stdout")?;
    let mut output = Vec::new();
    stdout
        .take(max_output_bytes as u64 + 1)
        .read_to_end(&mut output)
        .context("read secret backend stdout")?;
    if output.len() > max_output_bytes {
        bail!("secret backend output exceeded {} bytes", max_output_bytes);
    }
    String::from_utf8(output).context("decode secret backend stdout as UTF-8")
}
