// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub mod catalog;

/// Captured output from a one-shot privileged child.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PrivilegedCommandOutput {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

/// Whether one-shot privileged command execution is enabled on this host.
///
/// PR 1 uses an environment toggle; full datadog.yaml config arrives in PR 3.
pub fn enabled() -> bool {
    std::env::var("DD_PM_PRIVILEGED_COMMANDS_ENABLED")
        .is_ok_and(|v| matches!(v.as_str(), "1" | "true" | "TRUE"))
}
