use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::frameworks::laravel::LaravelParser;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::path_utils::{remove_file_path, is_rune_letter_at};

const ARTISAN_CONSOLE: &str = "artisan";

/// PHP detector for Laravel and general PHP applications
pub struct PhpDetector;

impl PhpDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(PhpDetector)
    }
}

impl Detector for PhpDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        let mut metadata = ServiceMetadata::new(String::new(), ServiceNameSource::CommandLine);
        
        // Look for datadog.service (e.g., php -ddatadog.service=service_name OR php -d datadog.service=service_name)
        for arg in args {
            if arg.contains("datadog.service=") {
                if let Some(eq_pos) = arg.find('=') {
                    let value = &arg[eq_pos + 1..];
                    if !value.is_empty() {
                        metadata.dd_service = Some(value.to_string());
                        break;
                    }
                }
            }
        }
        
        let mut prev_arg_is_flag = false;
        
        for arg in args {
            let has_flag_prefix = arg.starts_with('-');
            
            // If the previous argument was a flag, or the current arg is a flag, skip the argument. Otherwise, process it.
            if !prev_arg_is_flag && !has_flag_prefix {
                let base_path = remove_file_path(arg);
                if is_rune_letter_at(&base_path, 0) && base_path == ARTISAN_CONSOLE {
                    let mut laravel_parser = LaravelParser::new(ctx);
                    let app_name = laravel_parser.get_laravel_app_name(arg);
                    metadata.name = app_name;
                    metadata.source = ServiceNameSource::Laravel;
                    return Ok(Some(metadata));
                }
            }
            
            let includes_assignment = arg.contains('=');
            prev_arg_is_flag = has_flag_prefix && !includes_assignment;
        }
        
        // If we found DD_SERVICE but no Laravel app, return None to indicate no service detected
        if metadata.dd_service.is_some() && metadata.name.is_empty() {
            return Ok(Some(metadata));
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "php"
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
    fn test_php_detector_artisan_command() {
        let detector = PhpDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        // Add a fake .env file for Laravel
        fs.add_file("laravel/.env", b"APP_NAME=MyLaravelApp\n".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["php".to_string(), "artisan".to_string(), "serve".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        // Laravel detector will return "MyLaravelApp" or "laravel" as fallback
        assert!(metadata.name == "MyLaravelApp" || metadata.name == "laravel");
        assert_eq!(metadata.source, ServiceNameSource::Laravel);
    }

    #[test]
    fn test_php_detector_dd_service() {
        let detector = PhpDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["php".to_string(), "-ddatadog.service=service_name".to_string(), "server.php".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.dd_service, Some("service_name".to_string()));
    }

    #[test]
    fn test_php_detector_dd_service_with_space() {
        let detector = PhpDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["php".to_string(), "-d".to_string(), "datadog.service=service_name".to_string(), "server.php".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.dd_service, Some("service_name".to_string()));
    }

    #[test]
    fn test_php_detector_no_match() {
        let detector = PhpDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["php".to_string(), "server.php".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }

    #[test]
    fn test_php_detector_with_flags() {
        let detector = PhpDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        // Add a fake .env file for Laravel
        fs.add_file("laravel/.env", b"APP_NAME=TestApp\n".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["php".to_string(), "-x".to_string(), "a".to_string(), "artisan".to_string(), "serve".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert!(metadata.name == "TestApp" || metadata.name == "laravel");
        assert_eq!(metadata.source, ServiceNameSource::Laravel);
    }
}