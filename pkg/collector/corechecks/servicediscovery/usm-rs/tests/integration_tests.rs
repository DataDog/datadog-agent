use usm_rs::{
    context::{DetectionContext, Environment}, 
    filesystem::MemoryFileSystem, 
    language::Language,
    service::extract_service_metadata
};
use std::sync::Arc;

#[test]
fn test_basic_service_detection() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec!["java".to_string(), "-jar".to_string(), "myapp.jar".to_string()];
    let env = Environment::new();
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::Java, &mut ctx).unwrap();
    
    assert!(!metadata.name.is_empty());
    // Should fall back to command-line extraction since Java detector is stubbed
    assert_eq!(metadata.name, "myapp");
}

#[test]
fn test_dotnet_service_detection() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec!["dotnet".to_string(), "MyApplication.dll".to_string()];
    let env = Environment::new();
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::DotNet, &mut ctx).unwrap();
    
    assert_eq!(metadata.name, "MyApplication");
}

#[test]
fn test_simple_command_detection() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec!["myservice".to_string(), "--config".to_string(), "app.conf".to_string()];
    let env = Environment::new();
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
    
    assert_eq!(metadata.name, "myservice");
}

#[test]
fn test_dd_service_extraction() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec!["java".to_string(), "-jar".to_string(), "app.jar".to_string()];
    
    let mut env = Environment::new();
    env.set("DD_SERVICE".to_string(), "my-service-name".to_string());
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::Java, &mut ctx).unwrap();
    
    assert_eq!(metadata.dd_service, Some("my-service-name".to_string()));
}

#[test]
fn test_dd_tags_service_extraction() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec!["python".to_string(), "app.py".to_string()];
    
    let mut env = Environment::new();
    env.set("DD_TAGS".to_string(), "env:prod,service:tagged-service,version:1.0".to_string());
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::Python, &mut ctx).unwrap();
    
    assert_eq!(metadata.dd_service, Some("tagged-service".to_string()));
}

#[test]
fn test_empty_args() {
    let fs = Arc::new(MemoryFileSystem::new());
    let args = vec![];
    let env = Environment::new();
    
    let mut ctx = DetectionContext::new(args, env, fs);
    
    let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
    
    assert!(metadata.name.is_empty());
}

#[cfg(test)]
mod ffi_tests {
    use usm_rs::ffi::{usm_extract_service_metadata, usm_free_service_metadata, usm_version};
    use std::ffi::{CStr, CString};

    #[test]
    fn test_ffi_basic_extraction() {
        let args = [
            CString::new("java").unwrap(),
            CString::new("-jar").unwrap(),
            CString::new("myapp.jar").unwrap(),
        ];
        
        let arg_ptrs: Vec<*const i8> = args.iter().map(|s| s.as_ptr()).collect();
        
        let envs = [
            CString::new("DD_SERVICE").unwrap(),
            CString::new("test-service").unwrap(),
        ];
        
        let env_ptrs: Vec<*const i8> = envs.iter().map(|s| s.as_ptr()).collect();
        
        unsafe {
            let metadata = usm_extract_service_metadata(
                1, // Java
                1234, // PID
                arg_ptrs.as_ptr(),
                arg_ptrs.len() as i32,
                env_ptrs.as_ptr(),
                env_ptrs.len() as i32,
            );
            
            assert!(!metadata.is_null());
            
            let name = CStr::from_ptr((*metadata).name).to_str().unwrap();
            assert!(!name.is_empty());
            
            let dd_service = CStr::from_ptr((*metadata).dd_service).to_str().unwrap();
            assert_eq!(dd_service, "test-service");
            
            usm_free_service_metadata(metadata);
        }
    }
    
    #[test]
    fn test_ffi_version() {
        unsafe {
            let version_ptr = usm_version();
            let version = CStr::from_ptr(version_ptr).to_str().unwrap();
            assert!(version.starts_with("0.1.0"));
        }
    }
}