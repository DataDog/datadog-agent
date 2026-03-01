use core::{Aggregator, to_cstring, to_rust_string};
use std::{ffi::{CStr, c_char}, ptr};

use anyhow::{Ok, Result, bail};
use libloading::{Library, Symbol};

use crate::config::Config;

type FnRun = unsafe extern "C" fn(
    check_id: *const c_char,
    init_config: *const c_char,
    instance_config: *const c_char,
    aggregator: *const Aggregator,
    error: *mut *const c_char,
);

type FnVersion = unsafe extern "C" fn() -> *const c_char;

pub fn open(path: &str) -> Result<Library> {
    let handle = unsafe {
        Library::new(path)?
    };
    Ok(handle)
}

pub struct SharedLibrary<'a> {
    run: Symbol<'a, FnRun>,
    version: Symbol<'a, FnVersion>,
}

impl<'a> SharedLibrary<'a> {
    pub fn from_handle(handle: &'a Library) -> Result<Self> {
        
        let run = unsafe {
            handle.get("Run")?
        };
        let version = unsafe {
            handle.get("Version")?
        };

        Ok(Self { run, version })
    }

    pub fn run(&self, config_path: &str, aggregator: &Aggregator) -> Result<()> {
        let config = Config::extract(config_path)?;

        for instance_config in config.instances {
            self.run_instance("", &config.init_config, &instance_config, aggregator)?
        }

        Ok(())
    }

    fn run_instance(&self, check_id: &str, init_config: &str, instance_config: &str, aggregator: &Aggregator) -> Result<()> {
        let cstr_check_id = to_cstring(check_id)?;
        let cstr_init_config = to_cstring(init_config)?;
        let cstr_instance_config = to_cstring(instance_config)?;
        let error_ptr = ptr::null_mut();

        unsafe {
            (self.run)(
                cstr_check_id,
                cstr_init_config,
                cstr_instance_config,
                aggregator,
                error_ptr,
            )
        };

        if error_ptr.is_null() {
            return Ok(())
        }

        let error = unsafe {
            CStr::from_ptr(*error_ptr)
        }.to_str()?;
        bail!(error)
    }

    pub fn version(&self) -> Result<String> {
        let cstr_ptr = unsafe {
            (self.version)()
        };

        Ok(
            to_rust_string(cstr_ptr as *mut c_char)?
        )
    }
}
