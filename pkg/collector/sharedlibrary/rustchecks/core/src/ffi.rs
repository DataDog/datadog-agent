/// Macro used to generate all the check FFI code with the ACR-compatible ABI.
///
/// Generates the `check_run` entry point that matches the ACR's `CheckRunFn` signature,
/// and the `Version` symbol for check version reporting.
#[macro_export]
macro_rules! generate_ffi {
    ($check_function:ident, $version:ident) => {
        /// Entrypoint of the check, matching the ACR's `CheckRunFn` signature.
        ///
        /// # Safety
        ///
        /// All pointer arguments must be valid for the duration of this call:
        /// - `init_config_cstr`, `instance_config_cstr`, `enrichment_cstr`: valid, non-null,
        ///   NUL-terminated C strings.
        /// - `callback_ptr`: valid pointer to a `Callback` struct.
        /// - `ctx`: opaque pointer passed through to callbacks.
        /// - `error_handler`: valid, writable pointer; on error set to a heap-allocated string
        ///   that the caller must free.
        #[unsafe(no_mangle)]
        pub unsafe extern "C" fn check_run(
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            enrichment_cstr: *const std::ffi::c_char,
            callback_ptr: *const core::Callback,
            ctx: *mut std::ffi::c_void,
            error_handler: *mut *const std::ffi::c_char,
        ) {
            if let Err(e) = create_and_run_check(
                init_config_cstr,
                instance_config_cstr,
                enrichment_cstr,
                callback_ptr,
                ctx,
            ) {
                let error_msg = std::ffi::CString::new(e.to_string()).unwrap_or_default();
                unsafe {
                    let ptr = libc::strdup(error_msg.as_ptr());
                    *error_handler = ptr;
                };
            }
        }

        /// Build the check structure and execute its custom implementation
        fn create_and_run_check(
            init_config_cstr: *const std::ffi::c_char,
            instance_config_cstr: *const std::ffi::c_char,
            enrichment_cstr: *const std::ffi::c_char,
            callback_ptr: *const core::Callback,
            ctx: *mut std::ffi::c_void,
        ) -> Result<(), Box<dyn std::error::Error>> {
            // convert C args to Rust structs
            // Safety: pointers are guaranteed valid by the caller (check_run is unsafe extern "C")
            let init_config_str = unsafe { core::to_rust_string(init_config_cstr) }?;
            let init_config = core::Config::from_str(&init_config_str)?;

            let instance_config_str = unsafe { core::to_rust_string(instance_config_cstr) }?;
            let instance_config = core::Config::from_str(&instance_config_str)?;

            // parse enrichment data
            let enrichment_str = unsafe { core::to_rust_string(enrichment_cstr) }?;
            let enrichment = core::parse_enrichment(&enrichment_str)?;

            // create callback context from the host-provided callback struct and opaque ctx
            let callback = unsafe { core::CallbackContext::from_ptr(callback_ptr, ctx) };

            // create the check instance
            let agent_check =
                core::AgentCheck::new(init_config, instance_config, callback, enrichment);

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
