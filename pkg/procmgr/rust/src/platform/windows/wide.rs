// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::OsStr;
use std::os::windows::ffi::OsStrExt;

pub(crate) fn null_terminated(value: &str) -> Vec<u16> {
    OsStr::new(value).encode_wide().chain([0]).collect()
}
