// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

use std::ffi::CStr;
use std::os::raw::c_char;

#[unsafe(no_mangle)]
pub extern "C" fn dd_get_text_embeddings_print(input: *const c_char) {
    if input.is_null() {
        return;
    }

    let text = unsafe { CStr::from_ptr(input) }.to_string_lossy();
    println!("{text}");
}

