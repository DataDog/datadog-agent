// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

use std::ffi::{CStr, c_char};

use regex::Regex;

/// Print "Hello groovy world" to stdout.
#[unsafe(no_mangle)]
pub extern "C" fn tagmatcher_hello() {
    println!("Hello groovy funky");
}

/// Opaque handle to a compiled Rust regex.
pub struct TagMatcherRegex {
    inner: Regex,
}

/// Compile a NUL-terminated regex pattern.
///
/// Returns a heap-allocated `TagMatcherRegex` on success, or NULL if the
/// pattern is NULL, not valid UTF-8, or fails to compile.
/// The caller must free the returned pointer with `tagmatcher_regex_free`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn tagmatcher_regex_new(pattern: *const c_char) -> *mut TagMatcherRegex {
    if pattern.is_null() {
        return std::ptr::null_mut();
    }
    let cstr = unsafe { CStr::from_ptr(pattern) };
    let Ok(s) = cstr.to_str() else {
        return std::ptr::null_mut();
    };
    match Regex::new(s) {
        Ok(re) => Box::into_raw(Box::new(TagMatcherRegex { inner: re })),
        Err(_) => std::ptr::null_mut(),
    }
}

/// Return true if `haystack[..haystack_len]` matches the compiled regex.
///
/// Returns false if `re` or `haystack` is NULL, or the bytes are not valid UTF-8.
/// `haystack` does **not** need to be NUL-terminated; only `haystack_len` bytes
/// are read.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn tagmatcher_regex_is_match(
    re: *const TagMatcherRegex,
    haystack: *const c_char,
    haystack_len: usize,
) -> bool {
    if re.is_null() || haystack.is_null() {
        return false;
    }
    let re = unsafe { &*re };
    let bytes = unsafe { std::slice::from_raw_parts(haystack as *const u8, haystack_len) };
    let Ok(s) = std::str::from_utf8(bytes) else {
        return false;
    };
    re.inner.is_match(s)
}

/// Free a `TagMatcherRegex` previously returned by `tagmatcher_regex_new`.
///
/// Passing NULL is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn tagmatcher_regex_free(re: *mut TagMatcherRegex) {
    if re.is_null() {
        return;
    }
    // SAFETY: re was returned by tagmatcher_regex_new and has not been freed.
    let _ = unsafe { Box::from_raw(re) };
}
