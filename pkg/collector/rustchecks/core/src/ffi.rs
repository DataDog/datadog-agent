/// Macro used to generate all the check FFI code
#[macro_export]
macro_rules! generate_ffi {
    ($check_function:ident, $version:ident) => {
        use std::ffi;
        use std::error;

        /// Entrypoint of the check
        #[unsafe(no_mangle)]
        pub extern "C" fn Run(check_id_str: *const ffi::c_char, init_config_str: *const ffi::c_char, instance_config_str: *const ffi::c_char, aggregator_ptr: *const core::Aggregator, error_handler: *mut *mut ffi::c_char) {
            if let Err(e) = create_and_run_check(check_id_str, init_config_str, instance_config_str, aggregator_ptr) {
                let cstr_ptr = core::to_cstring(&e.to_string())
                    .unwrap_or(core::to_cstring("").unwrap());
                unsafe { *error_handler = cstr_ptr; };
            }
        }

        /// Build the check structure and execute its custom implementation
        fn create_and_run_check(check_id_str: *const ffi::c_char, init_config_str: *const ffi::c_char, instance_config_str: *const ffi::c_char, aggregator_ptr: *const core::Aggregator) -> Result<(), Box<dyn error::Error>> {
            // convert C args to Rust structs
            let check_id = core::to_rust_string(check_id_str)?;

            let init_config = core::to_rust_string(init_config_str)?;
            let instance_config = core::to_rust_string(instance_config_str)?;

            let aggregator = core::Aggregator::from_ptr(aggregator_ptr);

            // create the check instance
            let agent_check = core::AgentCheck::new(&check_id, &init_config, &instance_config, aggregator)?;

            // run the custom implementation
            $check_function(&agent_check)?;

            Ok(())
        }

        /// Get the version of the check
        #[unsafe(no_mangle)]
        pub extern "C" fn Version() -> *const ffi::c_char {
            $version.as_ptr()
        }
    }
}
