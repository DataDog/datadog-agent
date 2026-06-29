use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int};

use vrl::compiler::runtime::{Runtime, Terminate};
use vrl::compiler::{Program, TargetValue, TimeZone};
use vrl::value::{KeyString, ObjectMap, Secrets, Value};

pub struct VrlProgram {
    program: Program,
}

/// Compile a VRL boolean expression.
/// Returns a heap-allocated VrlProgram on success, or NULL on failure.
/// On failure, *err_out is set to a malloc'd error string (caller must free with vrl_free_string).
#[unsafe(no_mangle)]
pub extern "C" fn vrl_compile(source: *const c_char, err_out: *mut *mut c_char) -> *mut VrlProgram {
    if source.is_null() {
        return std::ptr::null_mut();
    }
    let source = match unsafe { CStr::from_ptr(source) }.to_str() {
        Ok(s) => s,
        Err(_) => return std::ptr::null_mut(),
    };

    match vrl::compiler::compile(source, &[]) {
        Ok(result) => Box::into_raw(Box::new(VrlProgram {
            program: result.program,
        })),
        Err(diag) => {
            if !err_out.is_null() {
                let msg = format!("{:?}", diag);
                if let Ok(s) = CString::new(msg) {
                    unsafe { *err_out = s.into_raw() };
                }
            }
            std::ptr::null_mut()
        }
    }
}

/// Evaluate the VRL program against a log message string (exposed as `.message`).
///
/// Returns:
///   1  — program evaluated to a truthy value (match)
///   0  — program evaluated to false/null, or called `abort` (no match)
///  -1  — runtime error; *err_out is set (caller must free with vrl_free_string)
#[unsafe(no_mangle)]
pub extern "C" fn vrl_eval(
    prog: *const VrlProgram,
    message: *const c_char,
    len: usize,
    err_out: *mut *mut c_char,
) -> c_int {
    if prog.is_null() || message.is_null() {
        return -1;
    }
    let prog = unsafe { &*prog };

    let msg_bytes = unsafe { std::slice::from_raw_parts(message as *const u8, len) };
    let msg_str = match std::str::from_utf8(msg_bytes) {
        Ok(s) => s,
        Err(_) => return -1,
    };

    let mut map = ObjectMap::new();
    map.insert(KeyString::from("message"), Value::from(msg_str));

    let mut target = TargetValue {
        value: Value::Object(map),
        metadata: Value::Object(ObjectMap::new()),
        secrets: Secrets::default(),
    };

    let timezone = TimeZone::default();
    let mut runtime = Runtime::default();

    match runtime.resolve(&mut target, &prog.program, &timezone) {
        Ok(Value::Boolean(true)) => 1,
        Ok(_) => 0,
        Err(Terminate::Abort(_)) => 0,
        Err(Terminate::Error(e)) => {
            if !err_out.is_null() {
                if let Ok(s) = CString::new(e.to_string()) {
                    unsafe { *err_out = s.into_raw() };
                }
            }
            -1
        }
    }
}

/// Free a VrlProgram returned by vrl_compile.
#[unsafe(no_mangle)]
pub extern "C" fn vrl_free_program(prog: *mut VrlProgram) {
    if !prog.is_null() {
        unsafe { drop(Box::from_raw(prog)) };
    }
}

/// Free a C string returned by this library.
#[unsafe(no_mangle)]
pub extern "C" fn vrl_free_string(s: *mut c_char) {
    if !s.is_null() {
        unsafe { drop(CString::from_raw(s)) };
    }
}
