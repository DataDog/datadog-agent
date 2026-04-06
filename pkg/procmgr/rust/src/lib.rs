// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub mod command;
pub mod config;
pub mod env;
pub mod grpc;
pub mod manager;
pub mod ordering;
pub mod platform;
pub mod process;
pub mod shutdown;
pub mod state;
#[cfg(any(test, feature = "test-helpers"))]
pub mod test_helpers;
pub mod uuid_gen;
