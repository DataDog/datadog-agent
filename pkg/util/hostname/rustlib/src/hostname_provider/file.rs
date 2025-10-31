use libc::c_char;
use std::ffi::{CStr, CString};

pub unsafe fn get_hostname(path_c: *const c_char) -> *mut c_char {
    if path_c.is_null() {
        return std::ptr::null_mut();
    }
    let c_path = CStr::from_ptr(path_c);
    let Ok(path) = c_path.to_str() else { return std::ptr::null_mut(); };
    if path.is_empty() {
        return std::ptr::null_mut();
    }
    if let Ok(bytes) = std::fs::read(path) {
        let s = String::from_utf8_lossy(&bytes).trim().to_string();
        if !s.is_empty() {
            if let Ok(c) = CString::new(s) {
                return c.into_raw();
            }
        }
    }
    std::ptr::null_mut()
}
