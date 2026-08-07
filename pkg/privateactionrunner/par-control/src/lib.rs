// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Process lifecycle, effective configuration, and identity bootstrap for the
//! split Private Action Runner control plane.

pub mod bootstrap;
pub mod config;
pub mod executor;
pub mod identity;
pub mod platform;
pub mod procmgr;
pub mod proto;
pub mod tls;
pub mod transport;

#[cfg(all(test, unix))]
pub mod test_support;
