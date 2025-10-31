use libc::c_char;
use std::ffi::{CString};
#[cfg(target_os = "windows")]
use windows_sys::Win32::Networking::WinSock::{gethostname, gethostbyname, HOSTENT};
#[cfg(not(target_os = "windows"))]
use std::process::Command;

#[cfg(not(target_os = "windows"))]
pub fn get_hostname() -> *mut c_char {
    // Linux/Unix: execute `/bin/hostname -f` like the Go implementation
    if let Ok(output) = Command::new("/bin/hostname").arg("-f").output() {
        if output.status.success() {
            let s = String::from_utf8_lossy(&output.stdout).trim().to_string();
            if !s.is_empty() {
                if let Ok(c) = CString::new(s) {
                    return c.into_raw();
                }
            }
        }
    }
    std::ptr::null_mut()
}

#[cfg(target_os = "windows")]
pub fn get_hostname() -> *mut c_char {
    // Windows: replicate Go code using GetHostByName on the current hostname
    unsafe {
        let mut buf = [0i8; 256];
        if gethostname(buf.as_mut_ptr(), buf.len() as i32) != 0 {
            return std::ptr::null_mut();
        }
        let hent = gethostbyname(buf.as_ptr());
        if hent.is_null() {
            return std::ptr::null_mut();
        }
        let name_ptr = (*(hent as *const HOSTENT)).h_name;
        if name_ptr.is_null() {
            return std::ptr::null_mut();
        }
        let cstr = CStr::from_ptr(name_ptr);
        if let Ok(s) = CString::new(cstr.to_bytes()) {
            return s.into_raw();
        }
        std::ptr::null_mut()
    }
}
