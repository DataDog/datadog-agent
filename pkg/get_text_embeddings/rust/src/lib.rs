// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

use std::ffi::CStr;
use std::os::raw::c_char;

#[unsafe(no_mangle)]
pub extern "C" fn dd_get_text_embeddings_init(_err: *mut *mut c_char) {
    println!("Init get_text_embeddings");
}

#[unsafe(no_mangle)]
pub extern "C" fn dd_get_text_embeddings_get_embeddings_size() -> usize {
    // TODO
    384
}

#[unsafe(no_mangle)]
pub extern "C" fn dd_get_text_embeddings_get_embeddings(text: *const c_char, _buffer: *mut f32, err: *mut *mut c_char) {
    if text.is_null() {
        unsafe {
            *err = std::ffi::CString::new("Failed to get embeddings: text is null").unwrap().into_raw();
        }
        return;
    }

    let text = unsafe { CStr::from_ptr(text) }.to_string_lossy();
    println!("{text}");
}

