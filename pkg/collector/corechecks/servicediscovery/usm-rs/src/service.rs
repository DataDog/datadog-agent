use crate::context::{choose_service_name_from_envs, service_name_injected, DetectionContext};
use crate::detectors::{DetectorFactory, DotnetDetector, JavaDetector, NodejsDetector, PhpDetector, PythonDetector, RubyDetector, SimpleDetector};
use crate::error::UsmResult;
use crate::language::Language;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::{is_rune_letter_at, normalize_exe_name, normalize_service_name, parse_exe_start_with_symbol, remove_file_path, trim_colon_right};
use std::collections::HashMap;

/// Map languages to their detector factories
fn get_language_detectors() -> HashMap<Language, DetectorFactory> {
    let mut detectors: HashMap<Language, DetectorFactory> = HashMap::new();
    detectors.insert(Language::Python, |ctx| PythonDetector::new(ctx));
    detectors.insert(Language::Ruby, |ctx| SimpleDetector::new(ctx));
    detectors.insert(Language::Java, |ctx| JavaDetector::new(ctx));
    detectors.insert(Language::Node, |ctx| NodejsDetector::new(ctx));
    detectors.insert(Language::DotNet, |ctx| DotnetDetector::new(ctx));
    detectors.insert(Language::PHP, |ctx| PhpDetector::new(ctx));
    detectors
}

/// Map executable names to detector factories for special handling
fn get_executable_detectors() -> HashMap<String, DetectorFactory> {
    let mut detectors: HashMap<String, DetectorFactory> = HashMap::new();
    detectors.insert("gunicorn".to_string(), |ctx| PythonDetector::new_gunicorn_detector(ctx));
    detectors.insert("puma".to_string(), |ctx| RubyDetector::new_rails_detector(ctx));
    detectors.insert("sudo".to_string(), |ctx| SimpleDetector::new(ctx));
    detectors
}

/// Extract service metadata from process information
pub fn extract_service_metadata(
    language: Language, 
    ctx: &mut DetectionContext
) -> UsmResult<ServiceMetadata> {
    let mut metadata = ServiceMetadata::default();
    
    if ctx.args.is_empty() || ctx.args[0].is_empty() {
        return Ok(metadata);
    }
    
    // Extract DD_SERVICE from environment variables
    if let Some(dd_service) = choose_service_name_from_envs(&ctx.envs) {
        let injected = service_name_injected(&ctx.envs);
        metadata.set_dd_service(Some(dd_service), injected);
    }
    
    let mut args = ctx.args.clone();
    let mut exe = args[0].clone();
    
    // Handle case where all args are packed into the first argument
    if args.len() == 1 {
        if let Some(space_idx) = exe.find(' ') {
            exe = exe[0..space_idx].to_string();
            args = exe.split(' ').map(|s| s.to_string()).collect();
        }
    }
    
    // Clean up the executable name
    exe = exe.trim_matches('"').to_string();
    exe = trim_colon_right(&remove_file_path(&exe));
    
    if !is_rune_letter_at(&exe, 0) {
        exe = parse_exe_start_with_symbol(&exe);
    }
    
    exe = normalize_exe_name(&exe);
    
    let language_detectors = get_language_detectors();
    let executable_detectors = get_executable_detectors();
    
    // Try executable-specific detector first
    if let Some(detector_factory) = executable_detectors.get(&exe) {
        let detector = detector_factory(ctx);
        if let Some(detected) = detector.detect(ctx, &args[1..])? {
            merge_detected_metadata(&mut metadata, detected);
            return Ok(metadata);
        }
    }
    
    // Try language-specific detector
    if let Some(detector_factory) = language_detectors.get(&language) {
        let detector = detector_factory(ctx);
        if let Some(detected) = detector.detect(ctx, &args[1..])? {
            merge_detected_metadata(&mut metadata, detected);
            return Ok(metadata);
        }
    }
    
    // Fall back to executable name as service name
    let service_name = if let Some(dot_pos) = exe.rfind('.') {
        // Remove file extension
        exe[..dot_pos].to_string()
    } else {
        exe
    };
    
    if !service_name.is_empty() {
        metadata.name = normalize_service_name(&service_name);
        metadata.source = ServiceNameSource::CommandLine;
    }
    
    Ok(metadata)
}

/// Merge detected metadata with existing metadata
fn merge_detected_metadata(existing: &mut ServiceMetadata, detected: ServiceMetadata) {
    // Use detected name and source
    existing.name = detected.name;
    existing.source = detected.source;
    existing.additional_names = detected.additional_names;
    
    // Keep DD_SERVICE from environment if not provided by detector
    if existing.dd_service.is_none() && detected.dd_service.is_some() {
        existing.dd_service = detected.dd_service;
        existing.dd_service_injected = detected.dd_service_injected;
    }
    
    // Merge metadata
    for (key, value) in detected.metadata {
        existing.metadata.insert(key, value);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context::Environment;
    use crate::filesystem::MemoryFileSystem;
    use std::sync::Arc;

    fn create_test_context(args: Vec<String>) -> DetectionContext {
        let fs = Arc::new(MemoryFileSystem::new());
        let env = Environment::new();
        DetectionContext::new(args, env, fs)
    }

    #[test]
    fn test_extract_service_metadata_simple() {
        let args = vec!["java".to_string(), "-jar".to_string(), "myapp.jar".to_string()];
        let mut ctx = create_test_context(args);
        
        let metadata = extract_service_metadata(Language::Java, &mut ctx).unwrap();
        
        assert!(!metadata.name.is_empty());
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_service_metadata_with_dd_service() {
        let args = vec!["java".to_string(), "-jar".to_string(), "app.jar".to_string()];
        let mut env = Environment::new();
        env.set("DD_SERVICE".to_string(), "my-service".to_string());
        
        let fs = Arc::new(MemoryFileSystem::new());
        let mut ctx = DetectionContext::new(args, env, fs);
        
        let metadata = extract_service_metadata(Language::Java, &mut ctx).unwrap();
        
        assert_eq!(metadata.dd_service, Some("my-service".to_string()));
    }

    #[test]
    fn test_extract_service_metadata_empty_args() {
        let mut ctx = create_test_context(vec![]);
        
        let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
        
        assert!(metadata.name.is_empty());
    }

    #[test]
    fn test_extract_service_metadata_fallback_to_exe() {
        let args = vec!["myservice".to_string()];
        let mut ctx = create_test_context(args);
        
        let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
        
        assert_eq!(metadata.name, "myservice");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_service_metadata_with_path() {
        let args = vec!["/usr/bin/myapp".to_string()];
        let mut ctx = create_test_context(args);
        
        let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
        
        assert_eq!(metadata.name, "myapp");
    }

    #[test]
    fn test_extract_service_metadata_with_extension() {
        let args = vec!["myservice.exe".to_string()];
        let mut ctx = create_test_context(args);
        
        let metadata = extract_service_metadata(Language::Unknown, &mut ctx).unwrap();
        
        assert_eq!(metadata.name, "myservice");
    }
}