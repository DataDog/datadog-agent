// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (`CreateProcessAsUserW`).

mod logon;
mod managed;
mod primary_token;
mod privileged;
mod stdio;
mod suspended;
pub(crate) mod user_profile;
pub(crate) mod win32;

pub(crate) use managed::spawn_child_handle;
