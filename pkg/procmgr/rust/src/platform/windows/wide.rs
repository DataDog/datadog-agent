// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::ffi::OsStr;
use std::os::windows::ffi::{OsStrExt, OsStringExt};

pub(crate) fn split_env_entry_wide(
    wide: &[u16],
) -> Option<(std::ffi::OsString, std::ffi::OsString)> {
    let eq = wide.iter().position(|&c| c == u16::from(b'='))?;
    let (k, v) = wide.split_at(eq);
    let v = &v[1..];
    if k.is_empty() {
        return None;
    }
    Some((
        std::ffi::OsString::from_wide(k),
        std::ffi::OsString::from_wide(v),
    ))
}

pub(crate) fn trim_wide_nul(wide: &[u16]) -> String {
    let end = wide.iter().position(|&c| c == 0).unwrap_or(wide.len());
    std::ffi::OsString::from_wide(&wide[..end])
        .to_string_lossy()
        .into_owned()
}

pub(crate) fn null_terminated(value: &str) -> Vec<u16> {
    OsStr::new(value).encode_wide().chain([0]).collect()
}

pub(crate) fn from_ptr(ptr: *const u16) -> String {
    if ptr.is_null() {
        return String::new();
    }
    unsafe {
        let len = (0..).take_while(|&i| *ptr.add(i) != 0).count();
        String::from_utf16_lossy(std::slice::from_raw_parts(ptr, len))
    }
}

#[cfg(test)]
pub(crate) fn from_slice(slice: &[u16]) -> String {
    String::from_utf16_lossy(slice)
}
