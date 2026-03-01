/// Macro used to generate all the check FFI code
#[macro_export]
macro_rules! generate_ffi {
    ($check_function:ident, $version:ident) => {
        /// Entrypoint of the check
        #[unsafe(no_mangle)]
        pub extern "C" fn Run(
            check_id_cstr: *const std::ffi::c_char,
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            aggregator_ptr: *const core::Aggregator,
            error_handler: *mut *mut std::ffi::c_char,
        ) {
            if let Err(e) = create_and_run_check(
                check_id_cstr,
                init_config_cstr,
                instance_config_cstr,
                aggregator_ptr,
            ) {
                let cstr_ptr = match core::to_cstring(&e.to_string()) {
                    Ok(ptr) => ptr,
                    Err(_) => core::to_cstring("").unwrap(),
                };

                unsafe {
                    *error_handler = cstr_ptr
                };
            }
        }

        /// Build the check structure and execute its custom implementation
        fn create_and_run_check(
            check_id_cstr: *const std::ffi::c_char,
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            aggregator_ptr: *const core::Aggregator,
        ) -> Result<(), Box<dyn std::error::Error>> {
            // convert C args to Rust structs
            let check_id = core::to_rust_string(check_id_cstr)?;

            let init_config_str = core::to_rust_string(init_config_cstr)?;
            let init_config = core::Config::from_str(&init_config_str)?;

            let instance_config_str = core::to_rust_string(instance_config_cstr)?;
            let instance_config = core::Config::from_str(&instance_config_str)?;

            let aggregator = core::Aggregator::from_ptr(aggregator_ptr);

            // create the check instance
            let agent_check =
                core::AgentCheck::new(check_id, init_config, instance_config, aggregator);

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
