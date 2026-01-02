use http_check::sink::shlib;
use http_check::sink::shlib::callback;
use http_check::{Result, anyhow};

use libc::c_char;
use std::ffi::{CStr, CString};
use std::str::FromStr;

fn str_from_ptr<'a>(ptr: *const c_char) -> Option<&'a str> {
    if ptr == std::ptr::null() {
        return None;
    };
    unsafe { CStr::from_ptr(ptr) }.to_str().ok()
}

fn run_impl(
    callback: *const callback::Callback,
    check_id: *const c_char,
    init_config: *const c_char,
    instance_config: *const c_char,
) -> Result<()> {
    let callback = unsafe { &*callback }; // FIXME check for nullptr
    let check_id = str_from_ptr(check_id).ok_or(anyhow!("invalid check_id"))?;
    let init_config = str_from_ptr(init_config).ok_or(anyhow!("invalid init_config"))?;
    let instance_config =
        str_from_ptr(instance_config).ok_or(anyhow!("invalid instance_config"))?;

    shlib::run(callback, check_id, init_config, instance_config)
}

//(char *, char *, char *, const aggregator_t *, const char **);
#[unsafe(no_mangle)]
pub extern "C" fn Run(
    check_id: *const c_char,
    init_config: *const c_char,
    instance_config: *const c_char,
    callback: *const callback::Callback,
    error: *mut *const c_char,
) {
    let res = run_impl(callback, check_id, init_config, instance_config);
    if let Err(err) = res {
        println!("Oopsie: {}", err.to_string());
        unsafe {
            *error = CString::from_str(err.to_string().as_str())
                .expect("allocation error")
                .into_raw()
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn Version(_error: *mut *const c_char) -> *const c_char {
    http_check::version::VERSION.as_ptr().cast()
}
