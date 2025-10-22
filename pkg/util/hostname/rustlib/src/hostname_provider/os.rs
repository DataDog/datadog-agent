use libc::c_char;
use std::ffi::CString;
use hostname as sys_hostname;

pub fn get_hostname() -> *mut c_char {
    if let Ok(name) = sys_hostname::get() {
        if let Ok(s) = name.into_string() {
            if let Ok(c) = CString::new(s) {
                return c.into_raw();
            }
        }
    }
    std::ptr::null_mut()
}

