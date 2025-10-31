use libc::{c_char,free};

mod hostname_provider;
mod validate;

pub use hostname_provider::{os_hostname, fqdn_hostname, file_hostname};

#[no_mangle]
pub extern "C" fn dd_hostname(provider: *const c_char, hostname_file: *const c_char) -> *mut c_char {
    if provider.is_null() {
        return std::ptr::null_mut();
    }
    let cstr = unsafe { std::ffi::CStr::from_ptr(provider) };
    let Ok(key) = cstr.to_str() else { return std::ptr::null_mut(); };
    match key {
        "os" => os_hostname(),
        "fqdn" => fqdn_hostname(),
        "file" => {
            let resolved_hostname = unsafe { file_hostname(hostname_file) };
            if resolved_hostname.is_null() {
                return std::ptr::null_mut();
            }
            // Validate without sanitizing; free on invalid to avoid leaks
            let cstr = unsafe { std::ffi::CStr::from_ptr(resolved_hostname) };
            let s = cstr.to_string_lossy();
            if validate::valid_hostname(s.as_ref()).is_ok() {
                return resolved_hostname;
            }
            unsafe { free(resolved_hostname as *mut _) };
            std::ptr::null_mut()
        },
        _ => std::ptr::null_mut(),
    }
}

#[no_mangle]
pub extern "C" fn dd_normalize_host(input: *const c_char) -> *mut c_char {
    if input.is_null() { return std::ptr::null_mut(); }
    let s = unsafe { std::ffi::CStr::from_ptr(input) }.to_string_lossy().to_string();
    match validate::normalize_host(&s) {
        Ok(n) => match std::ffi::CString::new(n) { Ok(c) => c.into_raw(), Err(_) => std::ptr::null_mut() },
        Err(_) => std::ptr::null_mut(),
    }
}

// 0 ok, 1 empty, 2 local, 3 too_long, 4 not_rfc
#[no_mangle]
pub extern "C" fn dd_valid_hostname_code(input: *const c_char) -> i32 {
    if input.is_null() { return 1; }
    let s = unsafe { std::ffi::CStr::from_ptr(input) }.to_string_lossy().to_string();
    if s.is_empty() { return 1; }
    let lower = s.to_ascii_lowercase();
    match lower.as_str() {
        "localhost" | "localhost.localdomain" | "localhost6.localdomain6" | "ip6-localhost" => return 2,
        _ => {}
    }
    if s.len() > 255 { return 3; }
    match validate::valid_hostname(&s) {
        Ok(()) => 0,
        Err(msg) => {
            match msg {
                "hostname is empty" => 1,
                "local hostname" => 2,
                "name exceeded the maximum length of 255 characters" => 3,
                _ => 4,
            }
        }
    }
}

#[no_mangle]
pub extern "C" fn dd_clean_hostname_dir(input: *const c_char) -> *mut c_char {
    if input.is_null() { return std::ptr::null_mut(); }
    let s = unsafe { std::ffi::CStr::from_ptr(input) }.to_string_lossy().to_string();
    let out = validate::clean_hostname_dir(&s);
    match std::ffi::CString::new(out) { Ok(c) => c.into_raw(), Err(_) => std::ptr::null_mut() }
}

// no sanitize: return raw provider results

#[no_mangle]
pub extern "C" fn dd_dll_free(ptr: *mut c_char) {
    if !ptr.is_null() {
        unsafe { free(ptr as *mut _) };
    }
}
