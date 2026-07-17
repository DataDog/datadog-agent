// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::os::windows::ffi::OsStrExt;

/// Encode a Rust string as a null-terminated UTF-16 vector for Win32 APIs.
pub(crate) fn null_terminated(s: impl AsRef<std::ffi::OsStr>) -> Vec<u16> {
    s.as_ref().encode_wide().chain([0]).collect()
}
