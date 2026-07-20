// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//! C ABI interface for compiling and evaluating VRL programs against log
//! messages, consumed by `pkg/logs/vrl`'s cgo bridge on the Go side.
//!
//! Exports:
//! - `vrl_compile` — compiles a VRL source string against this crate's
//!   curated [`functions`] library.
//! - `vrl_eval_filter` — evaluates a compiled program as a boolean predicate
//!   over `.message` (for `exclude_at_vrl_match`/`include_at_vrl_match`).
//! - `vrl_eval_transform` — runs a compiled program that mutates `.message`
//!   and returns the resulting bytes (for `mask_vrl`).
//! - `vrl_free_program` / `vrl_free_string` / `vrl_free_bytes` — release
//!   values returned by the functions above.
//!
//! All strings/byte buffers returned to the caller are length-delimited, not
//! NUL-terminated, since log content can contain embedded NULs.

#![allow(non_camel_case_types)] // C ABI types use C naming conventions

pub mod functions;

use std::ffi::{CStr, CString, c_char};
use std::panic::{self, AssertUnwindSafe};
use std::ptr;

use vrl::compiler::runtime::{Runtime, Terminate};
use vrl::compiler::{Program, TargetValue, TimeZone};
use vrl::value::{KeyString, ObjectMap, Secrets, Value};

// ---------------------------------------------------------------------------
// #[repr(C)] types
// ---------------------------------------------------------------------------

/// Opaque handle to a compiled VRL program.
pub struct vrl_program {
    program: Program,
}

/// Length-delimited byte buffer, heap-allocated by this library. Not
/// NUL-terminated. `data` is NULL with `len == 0` on failure.
#[repr(C)]
pub struct vrl_bytes {
    pub data: *mut u8,
    pub len: usize,
}

const EMPTY_BYTES: vrl_bytes = vrl_bytes {
    data: ptr::null_mut(),
    len: 0,
};

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

fn set_error(err_out: *mut *mut c_char, message: impl Into<String>) {
    if err_out.is_null() {
        return;
    }
    if let Ok(s) = CString::new(message.into()) {
        // SAFETY: caller guarantees err_out points to a valid, writable *mut c_char.
        unsafe { *err_out = s.into_raw() };
    }
}

fn bytes_to_out(bytes: &[u8]) -> vrl_bytes {
    let boxed: Box<[u8]> = bytes.into();
    let len = boxed.len();
    let data = Box::into_raw(boxed) as *mut u8;
    vrl_bytes { data, len }
}

fn target_from_message(message: &str) -> TargetValue {
    let mut map = ObjectMap::new();
    map.insert(KeyString::from("message"), Value::from(message));
    TargetValue {
        value: Value::Object(map),
        metadata: Value::Object(ObjectMap::new()),
        secrets: Secrets::default(),
    }
}

// SAFETY: caller guarantees `ptr` points to `len` valid, initialized bytes,
// or is null with `len == 0` (representing an empty message).
unsafe fn message_str<'a>(ptr: *const c_char, len: usize) -> Result<&'a str, &'static str> {
    if ptr.is_null() {
        if len != 0 {
            return Err("message pointer is null");
        }
        return Ok("");
    }
    let bytes = unsafe { std::slice::from_raw_parts(ptr as *const u8, len) };
    std::str::from_utf8(bytes).map_err(|_| "message is not valid UTF-8")
}

// ---------------------------------------------------------------------------
// exported functions
// ---------------------------------------------------------------------------

/// Compiles a VRL boolean expression or transform against this crate's
/// curated function list (see [`functions::function_list`]).
///
/// Returns a heap-allocated program on success, or NULL on failure. On
/// failure, if `err_out` is non-NULL, `*err_out` is set to a malloc'd error
/// string (caller must free with `vrl_free_string`).
///
/// # Safety
/// - `source` must be a valid, NUL-terminated C string.
/// - `err_out` may be NULL; if non-NULL it must point to a valid, writable
///   `*mut c_char`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_compile(
    source: *const c_char,
    err_out: *mut *mut c_char,
) -> *mut vrl_program {
    match panic::catch_unwind(AssertUnwindSafe(|| {
        if source.is_null() {
            return Err("source is null".to_string());
        }
        // SAFETY: caller guarantees source is a valid NUL-terminated C string.
        let source = match unsafe { CStr::from_ptr(source) }.to_str() {
            Ok(s) => s,
            Err(_) => return Err("source is not valid UTF-8".to_string()),
        };

        vrl::compiler::compile(source, &functions::function_list())
            .map(|result| {
                Box::into_raw(Box::new(vrl_program {
                    program: result.program,
                }))
            })
            .map_err(|diagnostics| format!("{diagnostics:?}"))
    })) {
        Ok(Ok(ptr)) => ptr,
        Ok(Err(message)) => {
            set_error(err_out, message);
            ptr::null_mut()
        }
        Err(_) => {
            set_error(err_out, "internal panic while compiling VRL program");
            ptr::null_mut()
        }
    }
}

/// Evaluates `prog` as a boolean predicate against `message` (exposed to the
/// VRL program as `.message`).
///
/// Returns:
///   1  — the program resolved to `true` (match)
///   0  — the program resolved to anything else, or called `abort` (no match)
///  -1  — a runtime error occurred; if `err_out` is non-NULL, `*err_out` is
///        set to a malloc'd error string (caller must free with `vrl_free_string`)
///
/// # Safety
/// - `prog` must be a valid pointer returned by `vrl_compile`, not yet freed.
/// - `message` must point to `message_len` valid, initialized bytes.
/// - `err_out` may be NULL; if non-NULL it must point to a valid, writable `*mut c_char`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_eval_filter(
    prog: *const vrl_program,
    message: *const c_char,
    message_len: usize,
    err_out: *mut *mut c_char,
) -> i32 {
    match panic::catch_unwind(AssertUnwindSafe(|| {
        if prog.is_null() {
            return Err("program is null".to_string());
        }
        // SAFETY: caller guarantees prog is a valid, non-null pointer from vrl_compile.
        let prog = unsafe { &*prog };

        // SAFETY: caller guarantees message points to message_len valid bytes.
        let message = unsafe { message_str(message, message_len) }?;

        let mut target = target_from_message(message);
        let mut runtime = Runtime::default();

        match runtime.resolve(&mut target, &prog.program, &TimeZone::default()) {
            Ok(Value::Boolean(true)) => Ok(1),
            Ok(_) => Ok(0),
            Err(Terminate::Abort(_)) => Ok(0),
            Err(Terminate::Error(e)) => Err(e.to_string()),
        }
    })) {
        Ok(Ok(result)) => result,
        Ok(Err(message)) => {
            set_error(err_out, message);
            -1
        }
        Err(_) => {
            set_error(err_out, "internal panic while evaluating VRL program");
            -1
        }
    }
}

/// Runs `prog` as a transform against `message` (exposed as `.message`) and
/// returns the resulting `.message` value, serialized to bytes.
///
/// On success, returns a non-empty `vrl_bytes` (caller must free with
/// `vrl_free_bytes`). On failure — a runtime error, an `abort`, or a program
/// that doesn't leave a `message` field on the target — returns an empty
/// `vrl_bytes` and, if `err_out` is non-NULL, sets `*err_out` to a malloc'd
/// error string (caller must free with `vrl_free_string`). Callers should
/// treat any failure the same way (this crate does not distinguish "aborted"
/// from "errored" for transforms, since both mean the transform cannot be
/// trusted to have run to completion).
///
/// # Safety
/// - `prog` must be a valid pointer returned by `vrl_compile`, not yet freed.
/// - `message` must point to `message_len` valid, initialized bytes.
/// - `err_out` may be NULL; if non-NULL it must point to a valid, writable `*mut c_char`.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_eval_transform(
    prog: *const vrl_program,
    message: *const c_char,
    message_len: usize,
    err_out: *mut *mut c_char,
) -> vrl_bytes {
    match panic::catch_unwind(AssertUnwindSafe(|| {
        if prog.is_null() {
            return Err("program is null".to_string());
        }
        // SAFETY: caller guarantees prog is a valid, non-null pointer from vrl_compile.
        let prog = unsafe { &*prog };

        // SAFETY: caller guarantees message points to message_len valid bytes.
        let message = unsafe { message_str(message, message_len) }?;

        let mut target = target_from_message(message);
        let mut runtime = Runtime::default();

        match runtime.resolve(&mut target, &prog.program, &TimeZone::default()) {
            Ok(_) => match target.value {
                Value::Object(ref map) => match map.get("message") {
                    Some(value) => Ok(value.coerce_to_bytes().to_vec()),
                    None => Err("transform removed the \"message\" field".to_string()),
                },
                _ => Err("transform replaced the root value with a non-object".to_string()),
            },
            Err(Terminate::Abort(_)) => Err("vrl program aborted".to_string()),
            Err(Terminate::Error(e)) => Err(e.to_string()),
        }
    })) {
        Ok(Ok(bytes)) => bytes_to_out(&bytes),
        Ok(Err(message)) => {
            set_error(err_out, message);
            EMPTY_BYTES
        }
        Err(_) => {
            set_error(err_out, "internal panic while evaluating VRL program");
            EMPTY_BYTES
        }
    }
}

/// Frees a program returned by `vrl_compile`.
///
/// # Safety
/// `prog` must be a pointer returned by `vrl_compile` and must not have been
/// freed before. Passing NULL is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_free_program(prog: *mut vrl_program) {
    if prog.is_null() {
        return;
    }
    let _ = panic::catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: caller guarantees prog was created by Box::into_raw in vrl_compile
        // and has not been freed before.
        drop(unsafe { Box::from_raw(prog) });
    }));
}

/// Frees a string returned by this library (`err_out` values).
///
/// # Safety
/// `s` must be a pointer previously set via this library's `err_out`
/// parameter and must not have been freed before. Passing NULL is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_free_string(s: *mut c_char) {
    if s.is_null() {
        return;
    }
    let _ = panic::catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: caller guarantees s was created by CString::into_raw in this
        // library and has not been freed before.
        drop(unsafe { CString::from_raw(s) });
    }));
}

/// Frees a byte buffer returned by `vrl_eval_transform`.
///
/// # Safety
/// `bytes` must have been returned by `vrl_eval_transform` and must not have
/// been freed before. Passing an empty/NULL buffer is a no-op.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vrl_free_bytes(bytes: vrl_bytes) {
    if bytes.data.is_null() {
        return;
    }
    let _ = panic::catch_unwind(AssertUnwindSafe(|| {
        // SAFETY: caller guarantees bytes.data/len came from Box::into_raw of a
        // Box<[u8]> of this exact length, created in bytes_to_out, and has not
        // been freed before.
        drop(unsafe { Box::from_raw(std::ptr::slice_from_raw_parts_mut(bytes.data, bytes.len)) });
    }));
}

#[cfg(test)]
mod tests {
    use super::*;

    fn compile(source: &str) -> *mut vrl_program {
        let c_source = CString::new(source).unwrap();
        let mut err: *mut c_char = ptr::null_mut();
        let prog = unsafe { vrl_compile(c_source.as_ptr(), &mut err) };
        if prog.is_null() && !err.is_null() {
            let msg = unsafe { CStr::from_ptr(err) }
                .to_string_lossy()
                .into_owned();
            unsafe { vrl_free_string(err) };
            panic!("compile failed: {msg}");
        }
        prog
    }

    #[test]
    fn filter_matches() {
        let prog = compile(r#"parse_json!(.message).level == "error""#);
        let msg = br#"{"level":"error"}"#;
        let mut err: *mut c_char = ptr::null_mut();
        let result =
            unsafe { vrl_eval_filter(prog, msg.as_ptr() as *const c_char, msg.len(), &mut err) };
        assert_eq!(result, 1);
        unsafe { vrl_free_program(prog) };
    }

    #[test]
    fn filter_no_match() {
        let prog = compile(r#"parse_json!(.message).level == "error""#);
        let msg = br#"{"level":"debug"}"#;
        let mut err: *mut c_char = ptr::null_mut();
        let result =
            unsafe { vrl_eval_filter(prog, msg.as_ptr() as *const c_char, msg.len(), &mut err) };
        assert_eq!(result, 0);
        unsafe { vrl_free_program(prog) };
    }

    #[test]
    fn filter_runtime_error_reports_negative_one() {
        let prog = compile(r#"parse_json!(.message).level == "error""#);
        let msg = b"not json";
        let mut err: *mut c_char = ptr::null_mut();
        let result =
            unsafe { vrl_eval_filter(prog, msg.as_ptr() as *const c_char, msg.len(), &mut err) };
        assert_eq!(result, -1);
        assert!(!err.is_null());
        unsafe { vrl_free_string(err) };
        unsafe { vrl_free_program(prog) };
    }

    #[test]
    fn transform_redacts_the_message() {
        let prog = compile(r#".message = redact!(.message, [r'\d+'])"#);
        let msg = b"my id is 123456";
        let mut err: *mut c_char = ptr::null_mut();
        let out =
            unsafe { vrl_eval_transform(prog, msg.as_ptr() as *const c_char, msg.len(), &mut err) };
        assert!(!out.data.is_null());
        let got = unsafe { std::slice::from_raw_parts(out.data, out.len) };
        assert_eq!(got, b"my id is [REDACTED]");
        unsafe { vrl_free_bytes(out) };
        unsafe { vrl_free_program(prog) };
    }

    #[test]
    fn transform_error_when_message_field_is_removed() {
        let prog = compile(r#". = {"other": "field"}"#);
        let msg = b"hello";
        let mut err: *mut c_char = ptr::null_mut();
        let out =
            unsafe { vrl_eval_transform(prog, msg.as_ptr() as *const c_char, msg.len(), &mut err) };
        assert!(out.data.is_null());
        assert!(!err.is_null());
        unsafe { vrl_free_string(err) };
        unsafe { vrl_free_program(prog) };
    }

    #[test]
    fn compile_error_is_reported() {
        let c_source = CString::new(".message ==").unwrap();
        let mut err: *mut c_char = ptr::null_mut();
        let prog = unsafe { vrl_compile(c_source.as_ptr(), &mut err) };
        assert!(prog.is_null());
        assert!(!err.is_null());
        unsafe { vrl_free_string(err) };
    }
}
