use crate::context::{ContextKey, DetectionContext};
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::{is_js_file, remove_file_extension};
use std::io::Read;
use std::path::{Path, PathBuf};

/// Node.js detector for package.json-based service detection
pub struct NodejsDetector;

impl NodejsDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(NodejsDetector)
    }
    
    /// Check if a file has JavaScript extension
    fn is_js(&self, filepath: &str) -> bool {
        is_js_file(filepath)
    }
    
    /// Find service name from the nearest package.json file walking up the directory tree
    fn find_name_from_nearest_package_json(&self, ctx: &mut DetectionContext, abs_file_path: &str) -> Option<String> {
        let mut current = PathBuf::from(abs_file_path);
        current.pop(); // Get parent directory
        
        loop {
            let package_json_path = current.join("package.json");
            let package_json_str = package_json_path.to_string_lossy();
            
            if let Some(name) = self.maybe_extract_service_name(ctx, &package_json_str) {
                if !name.is_empty() {
                    // Save package.json path for instrumentation detector
                    ctx.set_context(ContextKey::NodePackageJSONPath, package_json_str.to_string());
                    return Some(name);
                }
            }
            
            let parent = current.parent();
            if parent.is_none() || parent == Some(current.as_path()) {
                break;
            }
            current = parent.unwrap().to_path_buf();
        }
        
        None
    }
    
    /// Extract service name from package.json if it exists
    fn maybe_extract_service_name(&self, ctx: &DetectionContext, filename: &str) -> Option<String> {
        let path = Path::new(filename);
        
        // Try to open the file
        let file = ctx.filesystem.open(path).ok()?;
        
        // Read the file contents
        let mut reader = self.size_verified_reader(file).ok()?;
        let mut contents = String::new();
        reader.read_to_string(&mut contents).ok()?;
        
        // Parse JSON and extract name field
        let json_value: serde_json::Value = serde_json::from_str(&contents).ok()?;
        let name = json_value.get("name")?.as_str()?.to_string();
        
        Some(name)
    }
    
    /// Create a size-verified reader to prevent reading huge files
    fn size_verified_reader(&self, file: Box<dyn Read + Send>) -> UsmResult<Box<dyn Read + Send>> {
        const MAX_FILE_SIZE: usize = 1024 * 1024; // 1MB limit
        
        // Create a limited reader
        Ok(Box::new(file.take(MAX_FILE_SIZE as u64)))
    }
    
    /// Read symlink target if the path is a symlink
    fn read_symlink(&self, ctx: &DetectionContext, path: &Path) -> Option<String> {
        // Try to read symlink - this is a simplified version
        // In a real implementation, we might want to use the filesystem's readlink method
        ctx.filesystem.read_dir(path.parent()?).ok()?;
        None // Simplified - would need proper symlink support
    }
}

impl Detector for NodejsDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        let mut skip_next = false;
        
        for arg in args {
            if skip_next {
                skip_next = false;
                continue;
            }
            
            if arg.starts_with('-') {
                // Handle Node.js flags
                if arg == "-r" || arg == "--require" {
                    // Next arg can be a JS file but not the entry point, skip it
                    skip_next = !arg.contains('='); // If value is in same arg (--require=module), don't skip next
                    continue;
                }
                // Skip other flags
                continue;
            }
            
            // This is a potential entry point
            let abs_file = ctx.resolve_working_dir_relative_path(arg);
            let abs_file_str = abs_file.to_string_lossy();
            
            let entry_point = if self.is_js(arg) {
                // Direct JavaScript file
                Some(abs_file_str.to_string())
            } else if let Some(target) = self.read_symlink(ctx, &abs_file) {
                // Symlink to a JavaScript file
                if self.is_js(&target) {
                    let target_path = abs_file.parent()
                        .map(|p| p.join(&target))
                        .unwrap_or_else(|| PathBuf::from(&target));
                    Some(target_path.to_string_lossy().to_string())
                } else {
                    continue;
                }
            } else {
                continue;
            };
            
            if let Some(entry_point) = entry_point {
                // Check if the file exists
                if ctx.filesystem.exists(&abs_file) {
                    // Try to find package.json
                    if let Some(service_name) = self.find_name_from_nearest_package_json(ctx, &entry_point) {
                        return Ok(Some(ServiceMetadata::new(service_name, ServiceNameSource::Nodejs)));
                    }
                    
                    // Fall back to script name
                    let base_name = abs_file.file_name()
                        .and_then(|name| name.to_str())
                        .map(|name| remove_file_extension(name))
                        .unwrap_or_else(|| "node-app".to_string());
                    
                    return Ok(Some(ServiceMetadata::new(base_name, ServiceNameSource::CommandLine)));
                }
            }
        }
        
        Ok(None)
    }
    
    fn name(&self) -> &'static str {
        "nodejs"
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
    fn test_nodejs_detector_with_package_json() {
        let detector = NodejsDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        // Add package.json file
        let package_json = r#"{"name": "my-node-app", "version": "1.0.0"}"#;
        fs.add_file("app/package.json", package_json.as_bytes().to_vec());
        fs.add_file("app/index.js", b"console.log('hello')".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["app/index.js".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-node-app");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }

    #[test]
    fn test_nodejs_detector_fallback_to_filename() {
        let detector = NodejsDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        // Add JS file without package.json
        fs.add_file("server.js", b"console.log('hello')".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["server.js".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "server");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_nodejs_detector_skip_require_flags() {
        let detector = NodejsDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        fs.add_file("app.js", b"console.log('hello')".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec![
            "-r".to_string(),
            "dotenv/config".to_string(),
            "app.js".to_string(),
        ];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app");
    }

    #[test]
    fn test_nodejs_detector_javascript_extensions() {
        let detector = NodejsDetector;
        
        assert!(detector.is_js("app.js"));
        assert!(detector.is_js("module.mjs"));
        assert!(detector.is_js("script.cjs"));
        assert!(!detector.is_js("config.json"));
        assert!(!detector.is_js("app.py"));
    }

    #[test]
    fn test_nodejs_detector_no_javascript_file() {
        let detector = NodejsDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["config.json".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_none());
    }
}