/// Macro used to generate all the check FFI code
#[macro_export]
macro_rules! generate_ffi {
    ($check_function:ident, $version:ident) => {
        /// Entrypoint of the check
        #[allow(clippy::not_unsafe_ptr_arg_deref)]
        #[unsafe(no_mangle)]
        pub extern "C" fn Run(
            check_id_cstr: *const std::ffi::c_char,
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            aggregator_ptr: *const $crate::Aggregator,
            error_handler: *mut *mut std::ffi::c_char,
        ) {
            // Catch panics: unwinding across this `extern "C"` boundary would
            // abort (SIGABRT) the whole agent.
            // See https://doc.rust-lang.org/nomicon/ffi.html#catching-panic-preemptively
            let result = std::panic::catch_unwind(|| {
                create_and_run_check(
                    check_id_cstr,
                    init_config_cstr,
                    instance_config_cstr,
                    aggregator_ptr,
                )
                .map_err(|e| e.to_string())
            });

            let error_msg = match result {
                Ok(Ok(())) => return,
                Ok(Err(msg)) => msg,
                Err(payload) => {
                    let msg = payload
                        .downcast_ref::<&str>()
                        .map(|s| s.to_string())
                        .or_else(|| payload.downcast_ref::<String>().cloned())
                        .unwrap_or_else(|| "unknown panic".to_string());
                    format!("check panicked: {msg}")
                }
            };

            let error_cstr = std::ffi::CString::new(error_msg).unwrap_or_default();
            unsafe {
                let ptr = libc::strdup(error_cstr.as_ptr());
                *error_handler = ptr;
            };
        }

        /// Build the check structure and execute its custom implementation
        fn create_and_run_check(
            check_id_cstr: *const std::ffi::c_char,
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            aggregator_ptr: *const $crate::Aggregator,
        ) -> Result<(), Box<dyn std::error::Error>> {
            // convert C args to Rust structs
            let check_id = $crate::to_rust_string(check_id_cstr)?;

            let init_config_str = $crate::to_rust_string(init_config_cstr)?;
            let init_config = $crate::Config::from_str(&init_config_str)?;

            let instance_config_str = $crate::to_rust_string(instance_config_cstr)?;
            let instance_config = $crate::Config::from_str(&instance_config_str)?;

            let aggregator = $crate::Aggregator::from_ptr(aggregator_ptr);

            // create the check instance
            let agent_check =
                $crate::AgentCheck::new(check_id, init_config, instance_config, aggregator);

            // run the custom implementation
            $check_function(&agent_check)?;

            Ok(())
        }

        /// Get the version of the check
        #[unsafe(no_mangle)]
        pub extern "C" fn Version() -> *const std::ffi::c_char {
            $version.as_ptr()
        }
    };
}

#[cfg(test)]
mod tests {
    use crate::{AgentCheck, Aggregator, AggregatorStub};

    use anyhow::{Result, bail};

    use std::cell::Cell;
    use std::ffi::{CStr, CString, c_char};

    // Per-thread check behavior. A single `generate_ffi!` `Run` (one `#[no_mangle]`
    // symbol) is driven by each test via this thread-local.
    #[derive(Clone, Copy)]
    enum Behavior {
        Panic,
        Error,
        Success,
    }

    thread_local! {
        static BEHAVIOR: Cell<Behavior> = const { Cell::new(Behavior::Success) };
    }

    fn configurable_check(_check: &AgentCheck) -> Result<()> {
        match BEHAVIOR.with(Cell::get) {
            Behavior::Panic => panic!("divide by zero"),
            Behavior::Error => bail!("check failed"),
            Behavior::Success => Ok(()),
        }
    }

    const TEST_VERSION: &CStr = c"test";

    generate_ffi!(configurable_check, TEST_VERSION);

    /// Calls the generated `Run` and returns the reported error message
    /// (`strdup`-allocated, freed here), or `None` on success.
    fn run_check() -> Option<String> {
        let aggregator = AggregatorStub::new().aggregator();
        let check_id = CString::new("test-check").unwrap();
        let config = CString::new("{}").unwrap();
        let mut error_handler: *mut c_char = std::ptr::null_mut();

        Run(
            check_id.as_ptr(),
            config.as_ptr(),
            config.as_ptr(),
            &aggregator as *const Aggregator,
            &mut error_handler as *mut *mut c_char,
        );

        // Null means success: `Run` leaves `error_handler` untouched.
        if error_handler.is_null() {
            return None;
        }

        let msg = unsafe { CStr::from_ptr(error_handler) }
            .to_str()
            .unwrap()
            .to_string();
        unsafe { libc::free(error_handler as *mut libc::c_void) };
        Some(msg)
    }

    #[test]
    fn run_catches_panic_and_reports_it() {
        BEHAVIOR.with(|b| b.set(Behavior::Panic));

        let msg = run_check().expect("panicking check should report an error");
        assert_eq!(msg, "check panicked: divide by zero");
    }

    #[test]
    fn run_reports_check_error() {
        BEHAVIOR.with(|b| b.set(Behavior::Error));

        let msg = run_check().expect("failing check should report an error");
        assert_eq!(msg, "check failed");
    }

    #[test]
    fn run_succeeds_without_error() {
        BEHAVIOR.with(|b| b.set(Behavior::Success));

        assert!(
            run_check().is_none(),
            "successful check should not report an error"
        );
    }
}
