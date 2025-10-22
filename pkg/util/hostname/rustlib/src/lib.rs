use libc::{c_char,free};

mod hostname_provider;

pub use hostname_provider::os_hostname;

#[no_mangle]
pub extern "C" fn dd_dll_os_hostname() -> *mut c_char {
    return os_hostname();
}

#[no_mangle]
pub extern "C" fn dd_dll_free(ptr: *mut c_char) {
    if !ptr.is_null() {
        unsafe { free(ptr as *mut _) };
    }
}
