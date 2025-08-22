use crate::context::DetectionContext;
use crate::detectors::Detector;
use crate::error::UsmResult;
use crate::frameworks::gunicorn::{extract_gunicorn_name_from_args, extract_gunicorn_name_from_env};
use crate::metadata::{ServiceMetadata, ServiceNameSource};
use crate::utils::remove_file_extension;
use std::path::{Path, PathBuf};

const INIT_PY: &str = "__init__.py";
const ALL_PY_FILES: &str = "*.py";

/// Type of Python argument detected
#[derive(Debug, PartialEq)]
enum ArgType {
    None,
    Module,
    FileName,
}

/// Python detector for module and script detection
pub struct PythonDetector;

impl PythonDetector {
    pub fn new(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(PythonDetector)
    }
    
    pub fn new_gunicorn_detector(_ctx: &DetectionContext) -> Box<dyn Detector> {
        Box::new(GunicornDetector)
    }
    
    /// Parse Python command line arguments to find module name or filename
    fn parse_python_args(&self, args: &[String]) -> (ArgType, Option<String>) {
        let mut skip_next = false;
        let mut mod_next = false;
        
        for arg in args {
            if mod_next {
                return (ArgType::Module, Some(arg.clone()));
            }
            
            if skip_next {
                skip_next = false;
                continue;
            }
            
            if arg.starts_with("--") {
                // Long arguments
                if arg == "--check-hash-based-pycs" {
                    skip_next = true;
                }
            } else if arg.starts_with('-') {
                // Short arguments - process each character
                let chars: Vec<char> = arg[1..].chars().collect();
                for (idx, &ch) in chars.iter().enumerate() {
                    let rest = if idx + 1 < chars.len() {
                        chars[idx + 1..].iter().collect::<String>()
                    } else {
                        String::new()
                    };
                    
                    match ch {
                        'c' => {
                            // Everything after -c is a command and terminates option parsing
                            return (ArgType::None, None);
                        }
                        'm' => {
                            // Module name, either attached or in next arg
                            if !rest.is_empty() {
                                return (ArgType::Module, Some(rest));
                            }
                            mod_next = true;
                        }
                        'X' | 'W' => {
                            // Takes an argument
                            if rest.is_empty() {
                                skip_next = true;
                            }
                            break;
                        }
                        _ => {
                            // Other flags
                            continue;
                        }
                    }
                }
            } else {
                // Not a flag - this is a filename
                return (ArgType::FileName, Some(arg.clone()));
            }
        }
        
        (ArgType::None, None)
    }
    
    /// Deduce Python package name by walking up directories looking for __init__.py
    fn deduce_package_name(&self, ctx: &DetectionContext, dir_path: &str, filename: &str) -> Option<String> {
        let mut current = PathBuf::from(dir_path);
        let mut package_parts = Vec::new();
        
        loop {
            let init_py_path = current.join(INIT_PY);
            if !ctx.filesystem.exists(&init_py_path) {
                break;
            }
            
            if let Some(dir_name) = current.file_name().and_then(|n| n.to_str()) {
                package_parts.insert(0, dir_name.to_string());
            }
            
            if let Some(parent) = current.parent() {
                if parent == current {
                    break;
                }
                current = parent.to_path_buf();
            } else {
                break;
            }
        }
        
        if !package_parts.is_empty() && !filename.is_empty() {
            let file_name = remove_file_extension(filename);
            package_parts.push(file_name);
        }
        
        if !package_parts.is_empty() {
            Some(package_parts.join("."))
        } else {
            None
        }
    }
    
    /// Find the nearest top-level directory containing Python files
    fn find_nearest_top_level(&self, ctx: &DetectionContext, path: &str) -> String {
        let mut current = PathBuf::from(path);
        let mut last_valid = current.clone();
        
        loop {
            // Check if current directory contains Python files
            if let Ok(entries) = ctx.filesystem.glob(&current, ALL_PY_FILES) {
                if !entries.is_empty() {
                    last_valid = current.clone();
                }
            }
            
            if let Some(parent) = current.parent() {
                if parent == current {
                    break;
                }
                current = parent.to_path_buf();
            } else {
                break;
            }
        }
        
        let name = last_valid
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("python-app");
        
        remove_file_extension(name)
    }
    
    /// Detect uvicorn applications
    fn detect_uvicorn(&self, args: &[String]) -> Option<ServiceMetadata> {
        let mut skip_next = false;
        
        for arg in args {
            if skip_next {
                skip_next = false;
                continue;
            }
            
            if arg.starts_with('-') {
                if arg == "--header" {
                    // This takes an argument that looks like module:app
                    skip_next = true;
                }
                continue;
            }
            
            // Look for module:app pattern
            if arg.contains(':') {
                if let Some(module) = arg.split(':').next() {
                    return Some(ServiceMetadata::new(module.to_string(), ServiceNameSource::CommandLine));
                }
            }
        }
        
        Some(ServiceMetadata::new("uvicorn".to_string(), ServiceNameSource::CommandLine))
    }
}

impl Detector for PythonDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Check for Gunicorn or uvicorn
        if let Some(first_arg) = args.first() {
            let base = Path::new(first_arg)
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or("");
            
            if base == "gunicorn" || base == "gunicorn:" {
                let gunicorn = GunicornDetector;
                return gunicorn.detect(ctx, &args[1..]);
            }
            
            if base == "uvicorn" {
                if let Some(metadata) = self.detect_uvicorn(&args[1..]) {
                    return Ok(Some(metadata));
                }
            }
        }
        
        // Parse Python arguments
        let (arg_type, arg_value) = self.parse_python_args(args);
        
        match arg_type {
            ArgType::None => Ok(None),
            ArgType::Module => {
                if let Some(module) = arg_value {
                    Ok(Some(ServiceMetadata::new(module, ServiceNameSource::CommandLine)))
                } else {
                    Ok(None)
                }
            }
            ArgType::FileName => {
                if let Some(filename) = arg_value {
                    let abs_path = ctx.resolve_working_dir_relative_path(&filename);
                    
                    if !ctx.filesystem.exists(&abs_path) {
                        return Ok(None);
                    }
                    
                    let metadata = ctx.filesystem.metadata(&abs_path).ok();
                    let is_file = metadata.map_or(false, |m| m.is_file);
                    
                    let (dir_path, file_name) = if is_file {
                        if let Some(parent) = abs_path.parent() {
                            (parent.to_string_lossy().to_string(), 
                             abs_path.file_name().and_then(|n| n.to_str()).unwrap_or("").to_string())
                        } else {
                            // Root level file
                            let name = self.find_nearest_top_level(ctx, &filename);
                            return Ok(Some(ServiceMetadata::new(name, ServiceNameSource::CommandLine)));
                        }
                    } else {
                        (abs_path.to_string_lossy().to_string(), String::new())
                    };
                    
                    // Try to deduce package name
                    if let Some(package_name) = self.deduce_package_name(ctx, &dir_path, &file_name) {
                        return Ok(Some(ServiceMetadata::new(package_name, ServiceNameSource::Python)));
                    }
                    
                    // Fall back to directory name
                    let name = self.find_nearest_top_level(ctx, &dir_path);
                    
                    // Avoid generic names
                    let final_name = if [".", "/", "bin", "sbin"].contains(&name.as_str()) {
                        self.find_nearest_top_level(ctx, &file_name)
                    } else {
                        name
                    };
                    
                    Ok(Some(ServiceMetadata::new(final_name, ServiceNameSource::CommandLine)))
                } else {
                    Ok(None)
                }
            }
        }
    }
    
    fn name(&self) -> &'static str {
        "python"
    }
}

/// Gunicorn detector
pub struct GunicornDetector;

impl Detector for GunicornDetector {
    fn detect(&self, ctx: &mut DetectionContext, args: &[String]) -> UsmResult<Option<ServiceMetadata>> {
        // Check environment variables first
        if let Some(name) = extract_gunicorn_name_from_env(&ctx.envs) {
            return Ok(Some(ServiceMetadata::new(name, ServiceNameSource::Gunicorn)));
        }
        
        // Check command line arguments
        if let Some(name) = extract_gunicorn_name_from_args(args) {
            return Ok(Some(ServiceMetadata::new(name, ServiceNameSource::CommandLine)));
        }
        
        // Default to gunicorn
        Ok(Some(ServiceMetadata::new("gunicorn".to_string(), ServiceNameSource::CommandLine)))
    }
    
    fn name(&self) -> &'static str {
        "gunicorn"
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
    fn test_python_detector_module() {
        let detector = PythonDetector::new(&create_test_context());
        let mut ctx = create_test_context();
        
        let args = vec!["-m".to_string(), "myapp".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_python_detector_filename() {
        let detector = PythonDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        fs.add_file("app.py", b"print('hello')".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["app.py".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "python-app");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_python_detector_package() {
        let detector = PythonDetector::new(&create_test_context());
        let mut fs = MemoryFileSystem::new();
        
        // Create a Python package structure
        fs.add_file("mypackage/__init__.py", b"".to_vec());
        fs.add_file("mypackage/subpackage/__init__.py", b"".to_vec());
        fs.add_file("mypackage/subpackage/main.py", b"print('hello')".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let args = vec!["mypackage/subpackage/main.py".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "mypackage.subpackage.main");
        assert_eq!(metadata.source, ServiceNameSource::Python);
    }

    #[test]
    fn test_gunicorn_detector() {
        let detector = GunicornDetector;
        let mut ctx = create_test_context();
        
        let args = vec!["myapp:application".to_string()];
        let result = detector.detect(&mut ctx, &args).unwrap();
        
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
    }

    #[test]
    fn test_python_parse_args() {
        let detector = PythonDetector;
        
        // Test module parsing
        let (arg_type, value) = detector.parse_python_args(&["-m".to_string(), "django".to_string()]);
        assert_eq!(arg_type, ArgType::Module);
        assert_eq!(value, Some("django".to_string()));
        
        // Test attached module
        let (arg_type, value) = detector.parse_python_args(&["-mdjango".to_string()]);
        assert_eq!(arg_type, ArgType::Module);
        assert_eq!(value, Some("django".to_string()));
        
        // Test filename
        let (arg_type, value) = detector.parse_python_args(&["app.py".to_string()]);
        assert_eq!(arg_type, ArgType::FileName);
        assert_eq!(value, Some("app.py".to_string()));
        
        // Test -c command
        let (arg_type, _) = detector.parse_python_args(&["-c".to_string(), "print('hello')".to_string()]);
        assert_eq!(arg_type, ArgType::None);
    }

    #[test]
    fn test_detect_uvicorn() {
        let detector = PythonDetector;
        
        let result = detector.detect_uvicorn(&["myapp:application".to_string()]);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myapp");
    }
}