// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

#[cfg(any(test, windows))]
mod agent_password_logon;
mod profile;
mod request;
mod stdio;

#[cfg(windows)]
pub(crate) use agent_password_logon::resolve_agent_password_logon;
pub(crate) use profile::SpawnProfile;
pub(crate) use request::SpawnRequest;
#[cfg(windows)]
pub(crate) use stdio::StdioSetting;
