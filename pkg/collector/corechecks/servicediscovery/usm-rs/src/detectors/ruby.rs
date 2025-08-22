use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use regex::Regex;
use std::io::{BufRead, BufReader};
use std::path::PathBuf;

/// Ruby detector for Rails and general Ruby applications
pub struct RubyDetector;

impl RubyDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(RubyDetector)
    }
    
    pub fn new_rails_detector(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(RailsDetector)
    }
}

impl Detector for RubyDetector {
    fn detect(&self, _ctx: &mut DetectionContext, _args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Basic Ruby detector - could be enhanced with gem detection, etc.
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "ruby"
    }
}

/// Rails detector for Ruby on Rails applications
pub struct RailsDetector;

impl RailsDetector {
    /// Extract Puma name from command line arguments
    fn extract_puma_name(cmdline: &[String]) -> Option<ServiceMetadata> {
        if cmdline.is_empty() {
            return None;
        }
        
        let last_arg = &cmdline[cmdline.len() - 1];
        if last_arg.len() >= 2 && last_arg.starts_with('[') && last_arg.ends_with(']') {
            // Extract the name between the brackets
            let name = &last_arg[1..last_arg.len() - 1];
            return Some(ServiceMetadata::new(name.to_string(), ServiceNameSource::CommandLine));
        }
        
        None
    }
    
    /// Find Rails application name from config/application.rb
    fn find_rails_application_name(&self, ctx: &DetectionContext, filename: &str) -> Result<String, String> {
        let file = ctx.filesystem.open(std::path::Path::new(filename))
            .map_err(|e| format!("Could not open application.rb: {}", e))?;
        
        let reader = BufReader::new(file);
        let module_regex = Regex::new(r"module\s+([A-Z][a-zA-Z0-9_]*)")
            .map_err(|e| format!("Failed to compile regex: {}", e))?;
        
        for line in reader.lines() {
            let line = line.map_err(|e| format!("Failed to read line: {}", e))?;
            if let Some(captures) = module_regex.captures(&line) {
                if let Some(module_name) = captures.get(1) {
                    let pascal_cased = module_name.as_str();
                    let snake_cased = rails_underscore(pascal_cased);
                    return Ok(snake_cased);
                }
            }
        }
        
        Err("Could not find Ruby module name".to_string())
    }
    
    /// Detect Rails application
    fn detect_rails(&self, ctx: &mut DetectionContext) -> Option<ServiceMetadata> {
        // For this implementation, we'll use the working directory from the context
        // In a real implementation, you'd get the process working directory
        let cwd = ctx.get_working_dirs().first().cloned().unwrap_or_else(|| PathBuf::from("."));
        let abs_file = format!("{}/config/application.rb", cwd.display());
        
        if !ctx.filesystem.exists(std::path::Path::new(&abs_file)) {
            return None;
        }
        
        match self.find_rails_application_name(ctx, &abs_file) {
            Ok(name) => Some(ServiceMetadata::new(name, ServiceNameSource::Rails)),
            Err(_) => None,
        }
    }
}

impl Detector for RailsDetector {
    fn detect(&self, ctx: &mut DetectionContext, cmdline: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // First try Rails detection
        if let Some(metadata) = self.detect_rails(ctx) {
            return Ok(Some(metadata));
        }
        
        // If Rails detection fails, try to extract the name from Puma command line
        if let Some(metadata) = Self::extract_puma_name(cmdline) {
            return Ok(Some(metadata));
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "rails"
    }
}

/// Convert PascalCase to snake_case (Rails underscore method)
fn rails_underscore(pascal_cased: &str) -> String {
    // First pass: handle transitions from lowercase/digit to uppercase
    let match_first_cap = Regex::new(r"(.)([A-Z][a-z]+)").unwrap();
    let snake = match_first_cap.replace_all(pascal_cased, "${1}_${2}");
    
    // Second pass: handle transitions from lowercase/digit to uppercase
    let match_all_cap = Regex::new(r"([a-z0-9])([A-Z])").unwrap();
    let snake = match_all_cap.replace_all(&snake, "${1}_${2}");
    
    snake.to_lowercase()
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
        DetectionContext::new(vec![".".to_string()], env, fs)
    }

    #[test]
    fn test_rails_underscore() {
        assert_eq!(rails_underscore("Service"), "service");
        assert_eq!(rails_underscore("HTTPServer"), "http_server");
        assert_eq!(rails_underscore("HTTP2Server"), "http2_server");
        assert_eq!(rails_underscore("VeryLongServiceName"), "very_long_service_name");
        assert_eq!(rails_underscore("service_name"), "service_name");
        assert_eq!(rails_underscore(""), "");
    }

    #[test]
    fn test_extract_puma_name() {
        let cmdline = vec!["puma".to_string(), "[my-rails-app]".to_string()];
        let result = RailsDetector::extract_puma_name(&cmdline);
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-rails-app");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_puma_name_no_brackets() {
        let cmdline = vec!["puma".to_string(), "config.ru".to_string()];
        let result = RailsDetector::extract_puma_name(&cmdline);
        
        assert!(result.is_none());
    }

    #[test]
    fn test_detect_rails_application() {
        let detector = RailsDetector;
        let mut fs = MemoryFileSystem::new();
        
        // Add a fake config/application.rb file
        let app_rb_content = b"module RailsHello\n  class Application < Rails::Application\n  end\nend\n";
        fs.add_file("./config/application.rb", app_rb_content.to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![".".to_string()], env, filesystem);
        
        let result = detector.detect(&mut ctx, &[]).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "rails_hello");
        assert_eq!(metadata.source, ServiceNameSource::Rails);
    }

    #[test]
    fn test_detect_rails_with_acronym() {
        let detector = RailsDetector;
        let mut fs = MemoryFileSystem::new();
        
        // Test with acronym in module name
        let app_rb_content = b"module HTTPServer\n  class Application < Rails::Application\n  end\nend\n";
        fs.add_file("./config/application.rb", app_rb_content.to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![".".to_string()], env, filesystem);
        
        let result = detector.detect(&mut ctx, &[]).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "http_server");
        assert_eq!(metadata.source, ServiceNameSource::Rails);
    }

    #[test]
    fn test_detect_rails_no_application_rb() {
        let detector = RailsDetector;
        let mut ctx = create_test_context();
        
        let result = detector.detect(&mut ctx, &[]).unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn test_detect_rails_fallback_to_puma() {
        let detector = RailsDetector;
        let mut ctx = create_test_context();
        
        let cmdline = vec!["puma".to_string(), "[fallback-app]".to_string()];
        let result = detector.detect(&mut ctx, &cmdline).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "fallback-app");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }
}