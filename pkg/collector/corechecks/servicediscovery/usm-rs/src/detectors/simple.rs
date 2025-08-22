use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::{is_rune_letter_at, remove_file_path, trim_colon_right};

/// Simple detector that looks for the first non-flag argument as service name
pub struct SimpleDetector;

impl SimpleDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(SimpleDetector)
    }
}

impl Detector for SimpleDetector {
    fn detect(&self, _ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        let mut prev_arg_is_flag = false;
        
        for arg in args {
            let has_flag_prefix = arg.starts_with('-');
            let is_env_variable = arg.contains('=');
            let includes_assignment = arg.contains('=');
            let should_skip_arg = prev_arg_is_flag || has_flag_prefix || is_env_variable;
            
            if !should_skip_arg {
                let cleaned = trim_colon_right(&remove_file_path(arg));
                if is_rune_letter_at(&cleaned, 0) {
                    return Ok(Some(ServiceMetadata::new(cleaned, ServiceNameSource::CommandLine)));
                }
            }
            
            // A flag that doesn't include '=' means the next argument is its value
            prev_arg_is_flag = has_flag_prefix && !includes_assignment;
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "simple"
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context::Environment;
    use crate::filesystem::MemoryFileSystem;
    use std::sync::Arc;

    fn create_test_context() -> DetectionContext {
        let fs = Arc::new(MemoryFileSystem::new());
        let env = Environment::new();
        DetectionContext::new(vec![], env, fs)
    }

    #[test]
    fn test_simple_detector_basic() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["myapp".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_simple_detector_with_flags() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "--verbose".to_string(),
            "-jar".to_string(),
            "app.jar".to_string(),
            "--port".to_string(),
            "8080".to_string(),
            "service-name".to_string()
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "service-name");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_simple_detector_skip_env_vars() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "ENV_VAR=value".to_string(),
            "myapp".to_string()
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
    }

    #[test]
    fn test_simple_detector_no_valid_args() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "--verbose".to_string(),
            "123invalid".to_string(),
            "ENV=value".to_string()
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }

    #[test]
    fn test_simple_detector_with_path() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["/usr/bin/myservice".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myservice");
    }

    #[test]
    fn test_simple_detector_with_colon() {
        let detector = SimpleDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["service:tag".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "service");
    }
}