// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod credential;
mod inherit_supervisor;
mod managed;
mod startup_info_ex;
mod stdio;
mod suspended;
#[cfg(test)]
mod test_harness;
pub(crate) mod win32;

pub(crate) use credential::SpawnCredential;
pub(crate) use managed::{resolve_spawn_identity, spawn_child_handle};
