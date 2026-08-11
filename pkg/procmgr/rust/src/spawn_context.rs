// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//! Shared spawn error messages for platform backends.

pub(crate) fn failed_message(process_name: &str, command: &str) -> String {
    format!("[{process_name}] failed to spawn: {command}")
}
