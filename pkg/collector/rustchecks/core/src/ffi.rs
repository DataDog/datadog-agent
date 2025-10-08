/// Macro used to generate all the check FFI
#[macro_export]
macro_rules! generate_ffi {
    ($check_impl:ident) => {
        /// Entrypoint of the check
        #[unsafe(no_mangle)]
        pub extern "C" fn Run(check_id_str: *mut std::ffi::c_char, init_config_str: *mut std::ffi::c_char, instance_config_str: *mut std::ffi::c_char, aggregator_ptr: *mut rust_check_core::Aggregator, error_handler: *mut *mut std::ffi::c_char) {
            if let Err(e) = create_and_run_check(check_id_str, init_config_str, instance_config_str, aggregator_ptr) {
                // 
                let cstr_ptr = rust_check_core::to_cstring(&e.to_string())
                    .unwrap_or(rust_check_core::to_cstring("").unwrap());
                unsafe { *error_handler = cstr_ptr; };
            }
        }

        /// Build the check structure and execute its custom implementation
        fn create_and_run_check(check_id_str: *mut std::ffi::c_char, init_config_str: *mut std::ffi::c_char, instance_config_str: *mut std::ffi::c_char, aggregator_ptr: *mut rust_check_core::Aggregator) -> Result<(), Box<dyn std::error::Error>> {
            // convert C args to Rust structs
            let check_id = rust_check_core::to_rust_string(check_id_str)?;
            
            let init_config = rust_check_core::to_rust_string(init_config_str)?;
            let instance_config = rust_check_core::to_rust_string(instance_config_str)?;

            let aggregator = rust_check_core::Aggregator::from_ptr(aggregator_ptr);

            // create the check instance
            let check = rust_check_core::AgentCheck::new(&check_id, &init_config, &instance_config, aggregator)?;

            // run the custom implementation
            $check_impl(&check)
        }
    }
}
