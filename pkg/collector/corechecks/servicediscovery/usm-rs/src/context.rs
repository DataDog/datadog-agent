use crate::filesystem::FileSystem;
use indexmap::IndexMap;
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

/// Detection result that can be shared between detectors
#[derive(Debug, Clone)]
pub struct DetectionResult {
    pub success: bool,
    pub service_name: Option<String>,
    pub framework: Option<String>,
    pub additional_info: HashMap<String, String>,
}

impl Default for DetectionResult {
    fn default() -> Self {
        Self {
            success: false,
            service_name: None,
            framework: None,
            additional_info: HashMap::new(),
        }
    }
}

/// Framework detection hints to guide other detectors
#[derive(Debug, Clone)]
pub struct FrameworkHint {
    pub framework_type: FrameworkType,
    pub confidence: f32, // 0.0 to 1.0
    pub evidence: Vec<String>,
    pub suggested_service_name: Option<String>,
}

#[derive(Debug, Clone, PartialEq)]
pub enum FrameworkType {
    SpringBoot,
    Spring,
    NodeJs,
    React,
    Angular,
    Django,
    Flask,
    DotNetCore,
    AspNet,
    Rails,
    Sinatra,
    Laravel,
    Symfony,
    Express,
    FastAPI,
    Tomcat,
    JBoss,
    WebLogic,
    WebSphere,
    Unknown,
}

/// Key types for the detector context map
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum ContextKey {
    /// Path to Node.js package.json file
    NodePackageJSONPath,
    /// SubFS instance that the package.json path is valid in
    ServiceSubFS,
    /// Pointer to the Process instance of the service
    ServiceProc,
    /// Cached parsed JAR manifest
    JarManifest,
    /// Cached Spring Boot application properties
    SpringBootProperties,
    /// Detected framework information
    DetectedFramework,
    /// Cached working directory resolution
    ResolvedWorkingDir,
    /// Shared filesystem scan results
    FileSystemScanCache,
    /// Detected server vendor (JEE)
    ServerVendor,
    /// Context roots extracted from deployment descriptors
    ContextRoots,
    /// Cached configuration file contents
    ConfigFileCache,
    /// Inter-detector communication state
    DetectorState,
}

/// Environment variables container
#[derive(Debug, Clone, Default)]
pub struct Environment {
    vars: HashMap<String, String>,
}

impl Environment {
    /// Create new environment from key-value pairs
    pub fn new() -> Self {
        Self::default()
    }
    
    /// Create environment from HashMap
    pub fn from_map(vars: HashMap<String, String>) -> Self {
        Self { vars }
    }
    
    /// Get environment variable value
    pub fn get(&self, key: &str) -> Option<&str> {
        self.vars.get(key).map(|s| s.as_str())
    }
    
    /// Set environment variable
    pub fn set(&mut self, key: String, value: String) {
        self.vars.insert(key, value);
    }
    
    /// Check if environment variable exists and is non-empty
    pub fn has_non_empty(&self, key: &str) -> bool {
        self.get(key).map_or(false, |v| !v.is_empty())
    }
    
    /// Get all environment variables
    pub fn all(&self) -> &HashMap<String, String> {
        &self.vars
    }
}

/// Detection context for service metadata extraction
#[derive(Debug)]
pub struct DetectionContext {
    /// Process PID
    pub pid: u32,
    /// Command line arguments
    pub args: Vec<String>,
    /// Environment variables
    pub envs: Environment,
    /// File system interface
    pub filesystem: Arc<dyn FileSystem>,
    /// Context map for passing data between detectors
    pub context_map: IndexMap<ContextKey, Box<dyn std::any::Any + Send + Sync>>,
    /// Cached working directories to avoid repeated lookups
    cached_working_dirs: Option<Vec<PathBuf>>,
}

impl DetectionContext {
    /// Create new detection context
    pub fn new(
        args: Vec<String>,
        envs: Environment,
        filesystem: Arc<dyn FileSystem>,
    ) -> Self {
        Self {
            pid: 0,
            args,
            envs,
            filesystem,
            context_map: IndexMap::new(),
            cached_working_dirs: None,
        }
    }
    
    /// Create detection context with PID
    pub fn with_pid(
        pid: u32,
        args: Vec<String>,
        envs: Environment,
        filesystem: Arc<dyn FileSystem>,
    ) -> Self {
        Self {
            pid,
            args,
            envs,
            filesystem,
            context_map: IndexMap::new(),
            cached_working_dirs: None,
        }
    }
    
    /// Store data in the context map
    pub fn set_context<T: std::any::Any + Send + Sync>(&mut self, key: ContextKey, value: T) {
        self.context_map.insert(key, Box::new(value));
    }
    
    /// Retrieve data from the context map
    pub fn get_context<T: std::any::Any + Send + Sync>(&self, key: ContextKey) -> Option<&T> {
        self.context_map
            .get(&key)
            .and_then(|any| any.downcast_ref::<T>())
    }
    
    /// Get cached data or compute and store it
    pub fn get_or_compute<T, F>(&mut self, key: ContextKey, compute_fn: F) -> &T
    where
        T: std::any::Any + Send + Sync + Clone,
        F: FnOnce(&mut Self) -> T,
    {
        if !self.context_map.contains_key(&key) {
            let value = compute_fn(self);
            self.context_map.insert(key, Box::new(value));
        }
        
        self.context_map
            .get(&key)
            .unwrap()
            .downcast_ref::<T>()
            .unwrap()
    }
    
    /// Share detection results between detectors
    pub fn share_detection_result(&mut self, detector_name: &str, result: DetectionResult) {
        let mut shared_results: HashMap<String, DetectionResult> = 
            self.get_context(ContextKey::DetectorState)
                .cloned()
                .unwrap_or_default();
        
        shared_results.insert(detector_name.to_string(), result);
        self.set_context(ContextKey::DetectorState, shared_results);
    }
    
    /// Get detection result from another detector
    pub fn get_shared_detection_result(&self, detector_name: &str) -> Option<DetectionResult> {
        self.get_context::<HashMap<String, DetectionResult>>(ContextKey::DetectorState)
            .and_then(|results| results.get(detector_name).cloned())
    }
    
    /// Cache expensive filesystem operations
    pub fn cache_filesystem_scan(&mut self, path: &str, results: Vec<String>) {
        let mut cache: HashMap<String, Vec<String>> = 
            self.get_context(ContextKey::FileSystemScanCache)
                .cloned()
                .unwrap_or_default();
        
        cache.insert(path.to_string(), results);
        self.set_context(ContextKey::FileSystemScanCache, cache);
    }
    
    /// Get cached filesystem scan results
    pub fn get_cached_filesystem_scan(&self, path: &str) -> Option<&Vec<String>> {
        self.get_context::<HashMap<String, Vec<String>>>(ContextKey::FileSystemScanCache)
            .and_then(|cache| cache.get(path))
    }
    
    /// Cache configuration file contents to avoid re-reading
    pub fn cache_config_file(&mut self, path: &str, content: String) {
        let mut cache: HashMap<String, String> = 
            self.get_context(ContextKey::ConfigFileCache)
                .cloned()
                .unwrap_or_default();
        
        cache.insert(path.to_string(), content);
        self.set_context(ContextKey::ConfigFileCache, cache);
    }
    
    /// Get cached configuration file content
    pub fn get_cached_config_file(&self, path: &str) -> Option<&String> {
        self.get_context::<HashMap<String, String>>(ContextKey::ConfigFileCache)
            .and_then(|cache| cache.get(path))
    }
    
    /// Set framework detection hint for other detectors
    pub fn set_framework_hint(&mut self, framework: FrameworkHint) {
        self.set_context(ContextKey::DetectedFramework, framework);
    }
    
    /// Get framework hint from previous detection
    pub fn get_framework_hint(&self) -> Option<&FrameworkHint> {
        self.get_context(ContextKey::DetectedFramework)
    }
    
    /// Get candidate working directories
    pub fn get_working_dirs(&mut self) -> &[PathBuf] {
        if self.cached_working_dirs.is_none() {
            let mut candidates = Vec::new();
            
            // Try PWD environment variable first
            if let Some(pwd) = self.envs.get("PWD") {
                if !pwd.is_empty() {
                    candidates.push(PathBuf::from(pwd));
                }
            }
            
            // Try to get working directory from PID (platform-specific)
            if let Ok(cwd) = self.get_working_directory_from_pid() {
                if !cwd.as_os_str().is_empty() {
                    candidates.push(cwd);
                }
            }
            
            self.cached_working_dirs = Some(candidates);
        }
        
        self.cached_working_dirs.as_ref().unwrap()
    }
    
    /// Resolve a relative path using working directory candidates and enhanced filesystem support
    pub fn resolve_working_dir_relative_path(&mut self, path: &str) -> PathBuf {
        let working_dirs = self.get_working_dirs().to_vec();
        
        if working_dirs.is_empty() {
            // Fallback to current directory
            return self.filesystem
                .resolve_working_dir_relative(path, &PathBuf::from("."));
        }
        
        // Use filesystem's enhanced path resolution
        for cwd in &working_dirs {
            let resolved = self.filesystem.resolve_working_dir_relative(path, cwd);
            
            // Try to canonicalize the path if it exists
            if self.filesystem.exists(&resolved) {
                if let Ok(canonical) = self.filesystem.canonicalize(&resolved) {
                    return canonical;
                }
                return resolved;
            }
        }
        
        // Fall back to first candidate with filesystem resolution
        self.filesystem.resolve_working_dir_relative(path, &working_dirs[0])
    }
    
    /// Platform-specific working directory retrieval
    #[cfg(target_os = "linux")]
    fn get_working_directory_from_pid(&self) -> Result<PathBuf, std::io::Error> {
        use std::path::Path;
        
        if self.pid == 0 {
            return Err(std::io::Error::new(
                std::io::ErrorKind::InvalidInput,
                "No PID provided",
            ));
        }
        
        let cwd_path = format!("/proc/{}/cwd", self.pid);
        let target = std::fs::read_link(Path::new(&cwd_path))?;
        Ok(target)
    }
    
    #[cfg(not(target_os = "linux"))]
    fn get_working_directory_from_pid(&self) -> Result<PathBuf, std::io::Error> {
        Err(std::io::Error::new(
            std::io::ErrorKind::Unsupported,
            "Working directory from PID not supported on this platform",
        ))
    }
}

/// Extract environment variable value if it exists and is non-empty
pub fn extract_env_var(envs: &Environment, name: &str) -> Option<String> {
    envs.get(name)
        .filter(|v| !v.is_empty())
        .map(|v| v.to_string())
}

/// Check if service name was injected via DD_INJECTION_ENABLED
pub fn service_name_injected(envs: &Environment) -> bool {
    if let Some(env) = envs.get("DD_INJECTION_ENABLED") {
        env.split(',').any(|v| v == "service_name")
    } else {
        false
    }
}

/// Choose service name from standard Datadog environment variables
pub fn choose_service_name_from_envs(envs: &Environment) -> Option<String> {
    // Check DD_SERVICE first
    if let Some(service) = extract_env_var(envs, "DD_SERVICE") {
        return Some(service);
    }
    
    // Check DD_TAGS for service: tag
    if let Some(tags) = envs.get("DD_TAGS") {
        for tag in tags.split(',') {
            if let Some(service) = tag.strip_prefix("service:") {
                return Some(service.to_string());
            }
        }
    }
    
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::filesystem::MemoryFileSystem;

    #[test]
    fn test_environment() {
        let mut env = Environment::new();
        env.set("TEST_VAR".to_string(), "test_value".to_string());
        
        assert_eq!(env.get("TEST_VAR"), Some("test_value"));
        assert_eq!(env.get("NONEXISTENT"), None);
        assert!(env.has_non_empty("TEST_VAR"));
        assert!(!env.has_non_empty("NONEXISTENT"));
    }
    
    #[test]
    fn test_service_name_from_envs() {
        let mut env = Environment::new();
        env.set("DD_SERVICE".to_string(), "my-service".to_string());
        
        assert_eq!(choose_service_name_from_envs(&env), Some("my-service".to_string()));
        
        // Test DD_TAGS fallback
        let mut env2 = Environment::new();
        env2.set("DD_TAGS".to_string(), "env:prod,service:tag-service,version:1.0".to_string());
        
        assert_eq!(choose_service_name_from_envs(&env2), Some("tag-service".to_string()));
    }
    
    #[test]
    fn test_detection_context() {
        let fs = Arc::new(MemoryFileSystem::new());
        let args = vec!["java".to_string(), "-jar".to_string(), "app.jar".to_string()];
        let env = Environment::new();
        
        let mut ctx = DetectionContext::new(args.clone(), env, fs);
        assert_eq!(ctx.args, args);
        assert_eq!(ctx.pid, 0);
        
        // Test context map
        ctx.set_context(ContextKey::NodePackageJSONPath, "/path/to/package.json".to_string());
        assert_eq!(
            ctx.get_context::<String>(ContextKey::NodePackageJSONPath),
            Some(&"/path/to/package.json".to_string())
        );
    }
}