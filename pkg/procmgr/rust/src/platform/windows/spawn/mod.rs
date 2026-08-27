// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

mod credential;
mod logon;
mod managed;
mod primary_token;
mod privileged;
mod stdio;
mod suspended;
#[cfg(test)]
mod test_harness;
pub(crate) mod user_profile;
pub(crate) mod win32;

#[cfg(any(test, feature = "test-helpers"))]
pub(crate) use credential::SpawnCredential;
pub(crate) use managed::{abort_uncommitted_spawn, spawn_managed_child};
