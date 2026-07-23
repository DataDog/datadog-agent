// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Windows child spawning by spawn profile (`CreateProcessAsUserW` vs inherited LocalSystem).

mod command;
mod logon;
mod managed;
mod primary_token;
mod privileged;
mod profiles;
mod stdio;

pub(crate) use managed::spawn_child_handle;
