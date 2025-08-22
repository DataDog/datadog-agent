//! FFI interface for Go integration

use crate::context::{DetectionContext, Environment};
use crate::filesystem::RealFileSystem;
use crate::language::Language;
use crate::metadata::ServiceMetadata;
use crate::service::extract_service_metadata;
use std::collections::HashMap;
use std::ffi::{CStr, CString};
use std::os::raw::{c_char, c_int, c_uint};
use std::ptr;
use std::sync::Arc;

/// C-compatible service metadata structure
#[repr(C)]
pub struct CServiceMetadata {
    pub name: *mut c_char,
    pub source: *mut c_char,
    pub dd_service: *mut c_char,
    pub dd_service_injected: c_int,
    pub additional_names: *mut *mut c_char,
    pub additional_names_len: c_int,
}

impl CServiceMetadata {
    fn from_rust(metadata: ServiceMetadata) -> Result<Self, Box<dyn std::error::Error>> {
        let name = CString::new(metadata.name)?;
        let source = CString::new(metadata.source.as_str())?;
        let dd_service = if let Some(dd_svc) = metadata.dd_service {
            CString::new(dd_svc)?.into_raw()
        } else {
            ptr::null_mut()
        };

        // Convert additional names
        let additional_names_len = metadata.additional_names.len() as c_int;
        let mut additional_names_vec = Vec::new();
        for name in metadata.additional_names {
            additional_names_vec.push(CString::new(name)?.into_raw());
        }
        
        let additional_names = if additional_names_vec.is_empty() {
            ptr::null_mut()
        } else {
            let boxed = additional_names_vec.into_boxed_slice();
            Box::into_raw(boxed) as *mut *mut c_char
        };

        Ok(CServiceMetadata {
            name: name.into_raw(),
            source: source.into_raw(),
            dd_service,
            dd_service_injected: if metadata.dd_service_injected { 1 } else { 0 },
            additional_names,
            additional_names_len,
        })
    }
}

/// Extract service metadata from process information
/// 
/// # Safety
/// This function assumes all input pointers are valid and null-terminated
#[no_mangle]
pub unsafe extern "C" fn usm_extract_service_metadata(
    language: c_int,
    pid: c_uint,
    args: *const *const c_char,
    args_len: c_int,
    envs: *const *const c_char,
    envs_len: c_int,
) -> *mut CServiceMetadata {
    // Convert inputs
    let lang = match language {
        0 => Language::Unknown,
        1 => Language::Java,
        2 => Language::Python,
        3 => Language::Node,
        4 => Language::PHP,
        5 => Language::Ruby,
        6 => Language::DotNet,
        7 => Language::Go,
        8 => Language::Rust,
        9 => Language::Cpp,
        _ => Language::Unknown,
    };

    // Convert args
    let mut rust_args = Vec::new();
    if !args.is_null() && args_len > 0 {
        for i in 0..args_len {
            let arg_ptr = *args.offset(i as isize);
            if !arg_ptr.is_null() {
                if let Ok(arg_str) = CStr::from_ptr(arg_ptr).to_str() {
                    rust_args.push(arg_str.to_string());
                }
            }
        }
    }

    // Convert environment variables
    let mut env_map = HashMap::new();
    if !envs.is_null() && envs_len > 0 {
        for i in 0..(envs_len / 2) {
            let key_ptr = *envs.offset((i * 2) as isize);
            let val_ptr = *envs.offset((i * 2 + 1) as isize);
            
            if !key_ptr.is_null() && !val_ptr.is_null() {
                if let (Ok(key), Ok(val)) = (
                    CStr::from_ptr(key_ptr).to_str(),
                    CStr::from_ptr(val_ptr).to_str()
                ) {
                    env_map.insert(key.to_string(), val.to_string());
                }
            }
        }
    }

    let environment = Environment::from_map(env_map);
    let filesystem = Arc::new(RealFileSystem::new());
    let mut ctx = DetectionContext::with_pid(pid, rust_args, environment, filesystem);

    // Extract metadata
    match extract_service_metadata(lang, &mut ctx) {
        Ok(metadata) => {
            match CServiceMetadata::from_rust(metadata) {
                Ok(c_metadata) => Box::into_raw(Box::new(c_metadata)),
                Err(_) => ptr::null_mut(),
            }
        }
        Err(_) => ptr::null_mut(),
    }
}

/// Free service metadata structure
/// 
/// # Safety
/// This function assumes the pointer was allocated by usm_extract_service_metadata
#[no_mangle]
pub unsafe extern "C" fn usm_free_service_metadata(metadata: *mut CServiceMetadata) {
    if metadata.is_null() {
        return;
    }

    let metadata = Box::from_raw(metadata);

    // Free name
    if !metadata.name.is_null() {
        let _ = CString::from_raw(metadata.name);
    }

    // Free source
    if !metadata.source.is_null() {
        let _ = CString::from_raw(metadata.source);
    }

    // Free dd_service
    if !metadata.dd_service.is_null() {
        let _ = CString::from_raw(metadata.dd_service);
    }

    // Free additional names
    if !metadata.additional_names.is_null() && metadata.additional_names_len > 0 {
        let additional_names = Box::from_raw(std::slice::from_raw_parts_mut(
            metadata.additional_names,
            metadata.additional_names_len as usize,
        ));
        
        for name_ptr in additional_names.iter() {
            if !name_ptr.is_null() {
                let _ = CString::from_raw(*name_ptr);
            }
        }
    }
}

/// Get the library version
#[no_mangle]
pub extern "C" fn usm_version() -> *const c_char {
    static VERSION: &str = concat!(env!("CARGO_PKG_VERSION"), "\0");
    VERSION.as_ptr() as *const c_char
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::metadata::ServiceNameSource;

    #[test]
    fn test_c_service_metadata_conversion() {
        let metadata = ServiceMetadata {
            name: "test-service".to_string(),
            source: ServiceNameSource::CommandLine,
            additional_names: vec!["app1".to_string(), "app2".to_string()],
            dd_service: Some("my-service".to_string()),
            dd_service_injected: true,
            metadata: HashMap::new(),
        };

        let c_metadata = CServiceMetadata::from_rust(metadata).unwrap();
        
        unsafe {
            let name = CStr::from_ptr(c_metadata.name).to_str().unwrap();
            assert_eq!(name, "test-service");
            
            let source = CStr::from_ptr(c_metadata.source).to_str().unwrap();
            assert_eq!(source, "command-line");
            
            assert!(c_metadata.dd_service_injected == 1);
            assert_eq!(c_metadata.additional_names_len, 2);
        }
    }

    #[test]
    fn test_version() {
        let version_ptr = usm_version();
        let version = unsafe { CStr::from_ptr(version_ptr) };
        assert!(version.to_str().unwrap().starts_with("0.1.0"));
    }
}