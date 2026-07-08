// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::path::PathBuf;
use std::process::Stdio;

/// Platform-agnostic spawn inputs for procmgr managed processes.
///
/// The platform backend is responsible for translating this into the OS-
/// specific spawn mechanism (Unix: `Command::spawn`; Windows: impersonation or
/// primary-token APIs).
pub struct SpawnRequest {
    pub command: String,
    pub args: Vec<String>,
    pub env: Vec<(String, String)>,
    pub working_dir: Option<PathBuf>,
    pub stdout: Stdio,
    pub stderr: Stdio,
}

