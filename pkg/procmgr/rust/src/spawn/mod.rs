// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Managed-process spawn: config → [`SpawnRequest`] → platform backend.

mod identity;
mod profile;
mod request;
mod stdio_setting;

pub(crate) use identity::spawn_user_for;
#[cfg(windows)]
pub(crate) use profile::DATADOG_AGENT_PROCESS;
pub use profile::{SpawnProfile, profile_for};
pub use request::SpawnRequest;
pub(crate) use stdio_setting::StdioSetting;
