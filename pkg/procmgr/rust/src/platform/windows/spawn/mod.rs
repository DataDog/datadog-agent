// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawn.
//!
//! Entry: `ManagedProcess::spawn_child_handle` in `managed.rs` (from `try_spawn`).
//! Routing lives in `managed.rs`; shared `CreateProcess*` job attach and resume logic
//! in `create_process.rs`.
//!
//! Three paths (see `managed.rs`):
//! - **Privileged** and **agent inherit**: `CreateProcessW` via `inherit_supervisor`
//! - **Agent logon**: `LogonUser` + `CreateProcessAsUserW` via `primary_token`

mod create_process;
mod credential;
mod inherit_supervisor;
mod inputs;
mod logon;
mod managed;
mod primary_token;
mod privileged;
mod startup_info_ex;
mod stdio;
#[cfg(test)]
mod test_harness;
pub(crate) mod user_profile;
pub(crate) mod win32;

pub(crate) use credential::{SpawnCredential, resolve_initial_spawn_identity};
