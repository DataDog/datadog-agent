// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

use std::panic::{self, AssertUnwindSafe};
use std::ptr;

use crate::tokenizer::Tokenizer;

#[allow(non_camel_case_types)]
pub type dd_tokenizer = Tokenizer;

/// Create a tokenizer. max_eval_bytes: 0 = unlimited.
/// Returns NULL on failure (should never happen).
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dd_tokenizer_new(max_eval_bytes: usize) -> *mut dd_tokenizer {
    match panic::catch_unwind(AssertUnwindSafe(|| {
        Box::into_raw(Box::new(Tokenizer::new(max_eval_bytes)))
    })) {
        Ok(ptr) => ptr,
        Err(_) => ptr::null_mut(),
    }
}

/// Free a tokenizer. NULL is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dd_tokenizer_free(t: *mut dd_tokenizer) {
    if t.is_null() {
        return;
    }
    let _ = panic::catch_unwind(AssertUnwindSafe(|| {
        unsafe { drop(Box::from_raw(t)) };
    }));
}

/// Tokenize input bytes. Writes tokens and indices into caller-owned buffers.
///
/// Returns the number of tokens written (>= 0) on success,
/// or -1 if capacity is insufficient.
///
/// tokens_out:  caller-allocated buffer for token bytes (u8)
/// indices_out: caller-allocated buffer for start indices (i32)
/// capacity:    size of both buffers (in elements)
#[unsafe(no_mangle)]
pub unsafe extern "C" fn dd_tokenizer_tokenize(
    t: *mut dd_tokenizer,
    input: *const u8,
    input_len: usize,
    tokens_out: *mut u8,
    indices_out: *mut i32,
    capacity: usize,
) -> i32 {
    match panic::catch_unwind(AssertUnwindSafe(|| {
        if t.is_null() || (input.is_null() && input_len > 0) {
            return -1;
        }

        let tokenizer = unsafe { &*t };
        let input_slice = if input_len == 0 {
            &[]
        } else {
            unsafe { std::slice::from_raw_parts(input, input_len) }
        };

        let tokens_buf = unsafe { std::slice::from_raw_parts_mut(tokens_out, capacity) };
        let indices_buf = unsafe { std::slice::from_raw_parts_mut(indices_out, capacity) };

        let n = tokenizer.tokenize_into(input_slice, tokens_buf, indices_buf);
        if n == usize::MAX {
            return -1;
        }
        n as i32
    })) {
        Ok(result) => result,
        Err(_) => -1,
    }
}
