// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawn.
//!
//! Comments here are written for readers who are not familiar with Windows: we spell out
//! Win32 concepts (access tokens, job objects, user profiles) as they appear.
//!
//! Entry: `managed::spawn_child_handle` (called from `ManagedProcess::spawn`).
//!
//! Three paths, chosen by `SpawnProfile` and whether the supervisor token already
//! matches the installed agent account. All three attach the job at `CreateProcess*`
//! via `STARTUPINFOEX` + `PROC_THREAD_ATTRIBUTE_JOB_LIST`. Windows does not use
//! Tokio `Command::spawn`.
//!
//! - **Privileged** (`datadog-agent-process`) and **agent inherit**: child runs as
//!   dd-procmgrd. `CreateProcessW` in `inherit_supervisor` (not
//!   `CreateProcessAsUserW`: that API needs `SeIncreaseQuotaPrivilege`, which the
//!   installed agent account does not hold).
//! - **Agent logon**: supervisor token does not match the agent account. `LogonUser` +
//!   `CreateProcessAsUserW` in `primary_token`, with the user profile kept loaded
//!   until the child exits.
//!
//! Access tokens: <https://learn.microsoft.com/en-us/windows/win32/secauthz/access-tokens>

mod credential;
mod inherit_supervisor;
mod logon;
mod managed;
mod primary_token;
mod startup_info_ex;
mod stdio;
#[cfg(test)]
mod test_harness;
pub(crate) mod user_profile;
pub(crate) mod win32;

pub(crate) use credential::SpawnCredential;
pub(crate) use managed::{resolve_spawn_identity, spawn_child_handle};
