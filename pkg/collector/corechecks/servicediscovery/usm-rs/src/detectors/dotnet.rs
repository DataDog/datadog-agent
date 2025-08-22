use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::{get_file_extension, remove_file_extension, remove_file_path};

/// .NET detector for assembly DLL execution
pub struct DotnetDetector;

impl DotnetDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(DotnetDetector)
    }
}

impl Detector for DotnetDetector {
    fn detect(&self, _ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        for arg in args {
            // Skip flags
            if arg.starts_with('-') {
                continue;
            }
            
            // Check if this is a DLL file (when running assembly's dll, the cli must be executed without command)
            // https://learn.microsoft.com/en-us/dotnet/core/tools/dotnet-run#description
            if let Some(ext) = get_file_extension(arg) {
                if ext == "dll" {
                    let filename = remove_file_path(arg);
                    let service_name = remove_file_extension(&filename);
                    return Ok(Some(ServiceMetadata::new(service_name, ServiceNameSource::CommandLine)));
                }
            }
            
            // If the first non-flag argument is not a DLL file, exit early since
            // nothing is matching a DLL execute case
            break;
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "dotnet"
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
    fn test_dotnet_detector_dll_execution() {
        let detector = DotnetDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["MyApplication.dll".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "MyApplication");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_dotnet_detector_with_path() {
        let detector = DotnetDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["/app/publish/WebApi.dll".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "WebApi");
    }

    #[test]
    fn test_dotnet_detector_with_flags() {
        let detector = DotnetDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec![
            "--verbose".to_string(),
            "MyService.dll".to_string()
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "MyService");
    }

    #[test]
    fn test_dotnet_detector_non_dll() {
        let detector = DotnetDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["run".to_string(), "MyProject.csproj".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        // Should not detect anything since first non-flag arg is not a DLL
        assert!(result.is_none());
    }

    #[test]
    fn test_dotnet_detector_no_args() {
        let detector = DotnetDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args: Vec<String> = vec![];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }
}