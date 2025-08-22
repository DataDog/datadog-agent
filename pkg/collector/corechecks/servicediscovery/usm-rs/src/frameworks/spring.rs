// Spring Boot detection framework

use crate::context::DetectionContext;
use crate::error::UsmResult;
use crate::filesystem::ReadSeek;
use crate::utils::zip_utils;
use std::collections::HashMap;
use std::io::Read;

// Spring Boot constants matching Go implementation
const BOOT_INF_JAR_PATH: &str = "BOOT-INF/classes/";
const DEFAULT_LOCATIONS: &str = "optional:classpath:/;optional:classpath:/config/;optional:file:./;optional:file:./config/;optional:file:./config/*/";
const DEFAULT_CONFIG_NAME: &str = "application";
const LOCATION_PROP_NAME: &str = "spring.config.locations";
const CONFIG_PROP_NAME: &str = "spring.config.name";
const ACTIVE_PROFILES_PROP_NAME: &str = "spring.profiles.active";
const APPNAME_PROP_NAME: &str = "spring.application.name";
const SPRING_BOOT_LAUNCHER: &str = "org.springframework.boot.loader.launch.JarLauncher";
const SPRING_BOOT_OLD_LAUNCHER: &str = "org.springframework.boot.loader.JarLauncher";
const MANIFEST_FILE: &str = "META-INF/MANIFEST.MF";

/// Property source trait for Spring Boot configuration resolution
pub trait PropertySource {
    fn get(&self, key: &str) -> Option<String>;
    
    fn get_default(&self, key: &str, default_value: &str) -> String {
        self.get(key).unwrap_or_else(|| default_value.to_string())
    }
}

/// Property source backed by a HashMap
pub struct MapPropertySource {
    properties: HashMap<String, String>,
}

impl MapPropertySource {
    pub fn new(properties: HashMap<String, String>) -> Self {
        Self { properties }
    }
}

impl PropertySource for MapPropertySource {
    fn get(&self, key: &str) -> Option<String> {
        self.properties.get(key).cloned()
    }
}

/// Combined property source that checks multiple sources in order
pub struct CombinedPropertySource {
    sources: Vec<Box<dyn PropertySource>>,
}

impl CombinedPropertySource {
    pub fn new() -> Self {
        Self {
            sources: Vec::new(),
        }
    }
    
    pub fn add_source(&mut self, source: Box<dyn PropertySource>) {
        self.sources.push(source);
    }
}

impl PropertySource for CombinedPropertySource {
    fn get(&self, key: &str) -> Option<String> {
        for source in &self.sources {
            if let Some(value) = source.get(key) {
                return Some(value);
            }
        }
        None
    }
}

/// Property source from command line arguments
pub struct ArgumentPropertySource {
    properties: HashMap<String, String>,
}

impl ArgumentPropertySource {
    pub fn new(args: &[String], prefix: &str) -> Self {
        let mut properties = HashMap::new();
        
        for arg in args {
            if let Some(stripped) = arg.strip_prefix(prefix) {
                if let Some((key, value)) = stripped.split_once('=') {
                    properties.insert(key.to_string(), value.to_string());
                } else {
                    properties.insert(stripped.to_string(), String::new());
                }
            }
        }
        
        Self { properties }
    }
}

impl PropertySource for ArgumentPropertySource {
    fn get(&self, key: &str) -> Option<String> {
        self.properties.get(key).cloned()
    }
}

/// Property source from environment variables with Spring Boot normalization
pub struct EnvironmentPropertySource<'a> {
    ctx: &'a DetectionContext,
}

impl<'a> EnvironmentPropertySource<'a> {
    pub fn new(ctx: &'a DetectionContext) -> Self {
        Self { ctx }
    }
    
    /// Normalize property key to environment variable format
    /// Convert dots and dashes to underscores, make uppercase
    fn normalize_env_key(&self, key: &str) -> String {
        key.chars()
            .map(|c| match c {
                'a'..='z' => (c as u8 - b'a' + b'A') as char,
                'A'..='Z' | '0'..='9' => c,
                _ => '_',
            })
            .collect()
    }
}

impl<'a> PropertySource for EnvironmentPropertySource<'a> {
    fn get(&self, key: &str) -> Option<String> {
        let env_key = self.normalize_env_key(key);
        self.ctx.envs.get(&env_key).map(|s| s.to_string())
    }
}

/// Property expander that resolves placeholders like ${property.name:default}
/// Matching Go implementation's placeholder resolution behavior
pub struct PropertyExpander {
    source: Box<dyn PropertySource>,
}

impl PropertyExpander {
    pub fn new(source: impl PropertySource + 'static) -> Self {
        Self {
            source: Box::new(source),
        }
    }
    
    /// Resolve placeholders in a value string
    fn resolve_placeholders(&self, value: &str) -> String {
        let mut result = value.to_string();
        let mut changed = true;
        let mut iterations = 0;
        const MAX_ITERATIONS: usize = 10; // Prevent infinite loops
        
        // Keep resolving until no more placeholders are found or max iterations reached
        while changed && iterations < MAX_ITERATIONS {
            changed = false;
            iterations += 1;
            
            // Find placeholder patterns: ${key} or ${key:default}
            let mut new_result = String::new();
            let mut chars = result.chars().peekable();
            
            while let Some(c) = chars.next() {
                if c == '$' && chars.peek() == Some(&'{') {
                    chars.next(); // consume {
                    
                    // Extract placeholder content
                    let mut placeholder = String::new();
                    let mut brace_count = 1;
                    
                    while let Some(ch) = chars.next() {
                        if ch == '{' {
                            brace_count += 1;
                        } else if ch == '}' {
                            brace_count -= 1;
                            if brace_count == 0 {
                                break;
                            }
                        }
                        placeholder.push(ch);
                    }
                    
                    // Parse key and default value
                    let (key, default_value) = if let Some(colon_pos) = placeholder.find(':') {
                        (
                            &placeholder[..colon_pos],
                            Some(&placeholder[colon_pos + 1..])
                        )
                    } else {
                        (placeholder.as_str(), None)
                    };
                    
                    // Resolve the placeholder
                    if let Some(resolved_value) = self.source.get(key) {
                        new_result.push_str(&resolved_value);
                        changed = true;
                    } else if let Some(default) = default_value {
                        new_result.push_str(default);
                        changed = true;
                    } else {
                        // Keep original placeholder if not resolvable
                        new_result.push_str(&format!("${{{}}}", placeholder));
                    }
                } else {
                    new_result.push(c);
                }
            }
            
            result = new_result;
        }
        
        result
    }
}

impl PropertySource for PropertyExpander {
    fn get(&self, key: &str) -> Option<String> {
        if let Some(value) = self.source.get(key) {
            Some(self.resolve_placeholders(&value))
        } else {
            None
        }
    }
}

/// Spring Boot parser for extracting application names from Spring Boot archives
pub struct SpringBootParser<'a> {
    ctx: &'a mut DetectionContext,
}

impl<'a> SpringBootParser<'a> {
    pub fn new(ctx: &'a mut DetectionContext) -> Self {
        Self { ctx }
    }
    
    /// Get Spring Boot application name from a JAR file
    pub fn get_spring_boot_app_name(&mut self, jar_name: &str) -> UsmResult<Option<String>> {
        let abs_name = self.ctx.resolve_working_dir_relative_path(jar_name);
        
        if !self.ctx.filesystem.exists(&abs_name) {
            return Ok(None);
        }
        
        let file = self.ctx.filesystem.open_seekable(&abs_name)?;
        let mut reader = zip_utils::create_zip_reader(file)?;
        
        if !is_spring_boot_archive(&mut reader) {
            return Ok(None);
        }
        
        // Try to extract application name from properties files in the archive
        self.get_spring_boot_app_name_from_archive(&mut reader)
    }
    
    /// Get Spring Boot application name when launched via JarLauncher
    pub fn get_spring_boot_launcher_app_name(&mut self) -> UsmResult<Option<String>> {
        let class_path = get_class_path(&self.ctx.args);
        if class_path.is_empty() {
            return Ok(None);
        }
        
        // Use first classpath entry
        let base_path = self.ctx.resolve_working_dir_relative_path(&class_path[0]);
        
        if base_path.ends_with(".jar") {
            if let Ok(file) = self.ctx.filesystem.open_seekable(&base_path) {
                if let Ok(mut reader) = zip_utils::create_zip_reader(file) {
                    if let Ok(Some(name)) = self.get_spring_boot_app_name_from_archive(&mut reader) {
                        return Ok(Some(name));
                    }
                }
            }
        }
        
        Ok(None)
    }
    
    /// Extract application name from Spring Boot archive using comprehensive property resolution
    fn get_spring_boot_app_name_from_archive(&mut self, reader: &mut zip::ZipArchive<Box<dyn ReadSeek>>) -> UsmResult<Option<String>> {
        // Create combined property source with proper precedence (matching Go implementation)
        let mut combined = CombinedPropertySource::new();
        
        // Order matters for property precedence (first source wins):
        // 1. Command line arguments (--spring.application.name=value)
        combined.add_source(Box::new(PropertyExpander::new(ArgumentPropertySource::new(&self.ctx.args, "--"))));
        
        // 2. System properties (-Dspring.application.name=value)  
        combined.add_source(Box::new(PropertyExpander::new(ArgumentPropertySource::new(&self.ctx.args, "-D"))));
        
        // 3. Environment variables (SPRING_APPLICATION_NAME)
        let env_properties = self.extract_env_properties();
        combined.add_source(Box::new(PropertyExpander::new(MapPropertySource::new(env_properties))));
        
        // Check for direct property first (highest priority)
        if let Some(app_name) = combined.get(APPNAME_PROP_NAME) {
            return Ok(Some(app_name));
        }
        
        // Parse configuration files from JAR (matching Go implementation)
        let locations = combined.get_default(LOCATION_PROP_NAME, DEFAULT_LOCATIONS);
        let config_name = combined.get_default(CONFIG_PROP_NAME, DEFAULT_CONFIG_NAME);
        let profiles = self.parse_active_profiles(&combined);
        
        // Parse URI patterns for configuration file discovery
        let (_file_patterns, classpath_patterns) = self.parse_uri_patterns(&locations, &config_name, &profiles);
        
        // Scan JAR for matching configuration files
        if let Some(classpath_source) = self.scan_jar_for_config_files(reader, &classpath_patterns) {
            combined.add_source(classpath_source);
        }
        
        // Try to get application name from resolved configuration
        Ok(combined.get(APPNAME_PROP_NAME))
    }
    
    /// Parse active Spring profiles from property sources
    fn parse_active_profiles(&self, source: &CombinedPropertySource) -> Vec<String> {
        source.get(ACTIVE_PROFILES_PROP_NAME)
            .map(|profiles| profiles.split(',').map(|p| p.trim().to_string()).collect())
            .unwrap_or_default()
    }
    
    /// Parse Spring Boot configuration locations into file and classpath patterns
    fn parse_uri_patterns(&self, locations: &str, config_name: &str, profiles: &[String]) -> (Vec<String>, Vec<String>) {
        let mut file_patterns = Vec::new();
        let mut classpath_patterns = Vec::new();
        
        for location in locations.split(';') {
            let parts: Vec<&str> = location.split(':').collect();
            if parts.len() >= 2 {
                let is_classpath = parts[parts.len() - 2] == "classpath";
                let mut path = parts[parts.len() - 1].to_string();
                
                if is_classpath {
                    path = format!("{}{}", BOOT_INF_JAR_PATH, path);
                }
                
                if path.ends_with('/') {
                    // Directory - add all possible config file names with profiles
                    for profile in profiles {
                        for ext in &[".properties", ".yaml", ".yml"] {
                            let pattern = format!("{}{}-{}{}", path, config_name, profile, ext);
                            if is_classpath {
                                classpath_patterns.push(pattern);
                            } else {
                                file_patterns.push(pattern);
                            }
                        }
                    }
                    
                    // Add default config files (no profile)
                    for ext in &[".properties", ".yaml", ".yml"] {
                        let pattern = format!("{}{}{}", path, config_name, ext);
                        if is_classpath {
                            classpath_patterns.push(pattern);
                        } else {
                            file_patterns.push(pattern);
                        }
                    }
                } else {
                    // Direct file reference
                    if is_classpath {
                        classpath_patterns.push(path);
                    } else {
                        file_patterns.push(path);
                    }
                }
            }
        }
        
        (file_patterns, classpath_patterns)
    }
    
    /// Scan JAR archive for configuration files matching patterns
    fn scan_jar_for_config_files(&self, reader: &mut zip::ZipArchive<Box<dyn ReadSeek>>, patterns: &[String]) -> Option<Box<dyn PropertySource>> {
        let mut combined = CombinedPropertySource::new();
        let mut found_any = false;
        
        for pattern in patterns {
            for i in 0..reader.len() {
                if let Ok(mut file) = reader.by_index(i) {
                    if self.matches_pattern(file.name(), pattern) {
                        if let Some(source) = self.parse_config_file_from_zip(&mut file) {
                            combined.add_source(source);
                            found_any = true;
                        }
                    }
                }
            }
        }
        
        if found_any {
            Some(Box::new(combined))
        } else {
            None
        }
    }
    
    /// Advanced ant-style pattern matching (matching Go implementation)
    fn matches_pattern(&self, path: &str, pattern: &str) -> bool {
        // Handle exact matches first
        if path == pattern {
            return true;
        }
        
        // Convert ant-style pattern to regex-compatible pattern
        let regex_pattern = self.ant_pattern_to_regex(pattern);
        if let Ok(re) = regex::Regex::new(&regex_pattern) {
            return re.is_match(path);
        }
        
        // Fallback to simple suffix matching if regex fails
        pattern.contains('*') && path.ends_with(&pattern[pattern.rfind('*').unwrap() + 1..])
    }
    
    /// Convert ant-style pattern to regex pattern
    fn ant_pattern_to_regex(&self, pattern: &str) -> String {
        let mut regex_pattern = String::new();
        regex_pattern.push('^');
        
        let mut chars = pattern.chars().peekable();
        while let Some(c) = chars.next() {
            match c {
                '*' => {
                    if chars.peek() == Some(&'*') {
                        chars.next(); // consume second *
                        if chars.peek() == Some(&'/') {
                            chars.next(); // consume /
                            regex_pattern.push_str("(?:.*/)?"); // ** matches zero or more directories
                        } else {
                            regex_pattern.push_str(".*"); // ** at end matches anything
                        }
                    } else {
                        regex_pattern.push_str("[^/]*"); // * matches anything except /
                    }
                }
                '?' => {
                    regex_pattern.push_str("[^/]"); // ? matches single char except /
                }
                '[' => {
                    regex_pattern.push('[');
                    // Copy character class as-is
                    while let Some(ch) = chars.next() {
                        regex_pattern.push(ch);
                        if ch == ']' {
                            break;
                        }
                    }
                }
                '.' | '^' | '$' | '+' | '(' | ')' | '{' | '}' | '|' | '\\' => {
                    regex_pattern.push('\\');
                    regex_pattern.push(c);
                }
                _ => {
                    regex_pattern.push(c);
                }
            }
        }
        
        regex_pattern.push('$');
        regex_pattern
    }
    
    /// Parse configuration file from ZIP entry
    fn parse_config_file_from_zip(&self, file: &mut zip::read::ZipFile) -> Option<Box<dyn PropertySource>> {
        let mut content = String::new();
        if file.read_to_string(&mut content).is_err() {
            return None;
        }
        
        let filename = file.name().to_lowercase();
        if filename.ends_with(".properties") {
            self.parse_properties_content(&content)
        } else if filename.ends_with(".yml") || filename.ends_with(".yaml") {
            self.parse_yaml_content(&content)
        } else {
            None
        }
    }
    
    /// Parse Java properties format with placeholder resolution
    fn parse_properties_content(&self, content: &str) -> Option<Box<dyn PropertySource>> {
        #[cfg(feature = "properties")]
        {
            // Use java-properties crate for proper parsing
            if let Ok(properties) = java_properties::read(std::io::Cursor::new(content)) {
                let props_map: HashMap<String, String> = properties.into_iter().collect();
                return Some(Box::new(PropertyExpander::new(MapPropertySource::new(props_map))));
            }
        }
        
        // Fallback to manual parsing
        let mut properties = HashMap::new();
        
        for line in content.lines() {
            let line = line.trim();
            if line.is_empty() || line.starts_with('#') || line.starts_with('!') {
                continue;
            }
            
            // Handle both = and : separators, with proper escaping
            if let Some(sep_pos) = line.find(|c| c == '=' || c == ':') {
                let key = line[..sep_pos].trim();
                let value = line[sep_pos + 1..].trim();
                
                if !key.is_empty() {
                    // Unescape Java properties format
                    let unescaped_key = self.unescape_properties_string(key);
                    let unescaped_value = self.unescape_properties_string(value);
                    properties.insert(unescaped_key, unescaped_value);
                }
            }
        }
        
        Some(Box::new(PropertyExpander::new(MapPropertySource::new(properties))))
    }
    
    /// Unescape Java properties string (simplified)
    fn unescape_properties_string(&self, s: &str) -> String {
        s.replace("\\n", "\n")
         .replace("\\r", "\r")
         .replace("\\t", "\t")
         .replace("\\\\", "\\")
         .replace("\\=", "=")
         .replace("\\:", ":")
         .replace("\\#", "#")
         .replace("\\!", "!")
    }
    
    /// Parse YAML content with full serde_yaml support and placeholder resolution
    fn parse_yaml_content(&self, content: &str) -> Option<Box<dyn PropertySource>> {
        #[cfg(feature = "yaml")]
        {
            // Use serde_yaml for proper YAML parsing
            match serde_yaml::from_str::<serde_yaml::Value>(content) {
                Ok(yaml_value) => {
                    let mut properties = HashMap::new();
                    self.flatten_yaml_value(&yaml_value, String::new(), &mut properties);
                    Some(Box::new(PropertyExpander::new(MapPropertySource::new(properties))))
                }
                Err(_) => {
                    // Fallback to simplified parsing
                    self.parse_yaml_content_simple(content)
                }
            }
        }
        #[cfg(not(feature = "yaml"))]
        {
            self.parse_yaml_content_simple(content)
        }
    }
    
    /// Simplified YAML parsing fallback
    fn parse_yaml_content_simple(&self, content: &str) -> Option<Box<dyn PropertySource>> {
        let mut properties = HashMap::new();
        let mut current_path = Vec::new();
        
        for line in content.lines() {
            let trimmed = line.trim();
            if trimmed.is_empty() || trimmed.starts_with('#') {
                continue;
            }
            
            // Calculate indentation level
            let indent_level = (line.len() - line.trim_start().len()) / 2;
            
            // Adjust current path based on indentation
            while current_path.len() > indent_level {
                current_path.pop();
            }
            
            if let Some((key, value)) = trimmed.split_once(':') {
                let key = key.trim();
                let value = value.trim();
                
                if !value.is_empty() {
                    // Leaf value
                    current_path.push(key.to_string());
                    let full_key = current_path.join(".");
                    properties.insert(full_key, value.trim_matches('"').trim_matches('\'').to_string());
                    current_path.pop();
                } else {
                    // Section header
                    current_path.push(key.to_string());
                }
            }
        }
        
        Some(Box::new(PropertyExpander::new(MapPropertySource::new(properties))))
    }
    
    /// Recursively flatten YAML value into dot-notation properties
    #[cfg(feature = "yaml")]
    fn flatten_yaml_value(&self, value: &serde_yaml::Value, prefix: String, properties: &mut HashMap<String, String>) {
        match value {
            serde_yaml::Value::Mapping(mapping) => {
                for (key, val) in mapping {
                    if let serde_yaml::Value::String(key_str) = key {
                        let new_prefix = if prefix.is_empty() {
                            key_str.clone()
                        } else {
                            format!("{}.{}", prefix, key_str)
                        };
                        self.flatten_yaml_value(val, new_prefix, properties);
                    }
                }
            }
            serde_yaml::Value::String(s) => {
                properties.insert(prefix, s.clone());
            }
            serde_yaml::Value::Number(n) => {
                properties.insert(prefix, n.to_string());
            }
            serde_yaml::Value::Bool(b) => {
                properties.insert(prefix, b.to_string());
            }
            serde_yaml::Value::Sequence(seq) => {
                for (i, item) in seq.iter().enumerate() {
                    let indexed_key = format!("{}[{}]", prefix, i);
                    self.flatten_yaml_value(item, indexed_key, properties);
                }
            }
            _ => {}
        }
    }
    
    /// Extract environment variables relevant to Spring Boot
    fn extract_env_properties(&self) -> HashMap<String, String> {
        let mut properties = HashMap::new();
        
        // Map Spring Boot properties from environment variables
        let spring_env_mappings = [
            ("SPRING_APPLICATION_NAME", "spring.application.name"),
            ("SPRING_CONFIG_NAME", "spring.config.name"),  
            ("SPRING_CONFIG_LOCATION", "spring.config.location"),
            ("SPRING_PROFILES_ACTIVE", "spring.profiles.active"),
        ];
        
        for (env_key, prop_key) in &spring_env_mappings {
            if let Some(value) = self.ctx.envs.get(*env_key) {
                properties.insert(prop_key.to_string(), value.to_string());
            }
        }
        
        properties
    }
}

/// Extract Spring Boot application name from properties content
fn extract_spring_application_name_from_content(content: &str, filename: &str) -> Option<String> {
    if filename.ends_with(".properties") {
        // Parse Java properties format
        for line in content.lines() {
            let line = line.trim();
            if let Some(value) = line.strip_prefix(&format!("{}=", APPNAME_PROP_NAME)) {
                let value = value.trim();
                if !value.is_empty() {
                    return Some(value.to_string());
                }
            }
        }
    } else if filename.ends_with(".yml") || filename.ends_with(".yaml") {
        // Simple YAML parsing for spring.application.name
        let mut in_spring_section = false;
        let mut in_application_section = false;
        
        for line in content.lines() {
            let trimmed = line.trim();
            
            if trimmed == "spring:" {
                in_spring_section = true;
                in_application_section = false;
                continue;
            }
            
            if in_spring_section {
                if trimmed.starts_with("application:") {
                    in_application_section = true;
                    continue;
                }
                
                if in_application_section && trimmed.starts_with("name:") {
                    if let Some(name) = trimmed.strip_prefix("name:") {
                        let name = name.trim().trim_matches('"').trim_matches('\'');
                        if !name.is_empty() {
                            return Some(name.to_string());
                        }
                    }
                }
                
                // Reset if we encounter a different top-level key
                if !trimmed.starts_with(' ') && !trimmed.starts_with('\t') && !trimmed.is_empty() && !trimmed.starts_with("application:") {
                    in_spring_section = false;
                    in_application_section = false;
                }
            }
        }
    }
    
    None
}

/// Extract Spring Boot application name from properties
pub fn extract_spring_application_name(properties: &HashMap<String, String>) -> Option<String> {
    // Check for Spring application name properties
    let spring_keys = [
        "spring.application.name",
        "spring.cloud.application.name",
        "info.app.name",
    ];
    
    for key in &spring_keys {
        if let Some(name) = properties.get(*key) {
            if !name.trim().is_empty() {
                return Some(name.trim().to_string());
            }
        }
    }
    
    None
}

/// Check if this appears to be a Spring Boot application
pub fn is_spring_boot_application(main_class: &str) -> bool {
    main_class.contains("org.springframework.boot.loader") ||
    main_class.contains("SpringApplication") ||
    main_class.ends_with("Application")
}

/// Check if a main class is a Spring Boot launcher (matching Go implementation)
pub fn is_spring_boot_launcher(main_class: &str) -> bool {
    main_class == SPRING_BOOT_LAUNCHER || main_class == SPRING_BOOT_OLD_LAUNCHER
}

/// Check if a ZIP archive is a Spring Boot fat JAR
pub fn is_spring_boot_archive(reader: &mut zip::ZipArchive<Box<dyn ReadSeek>>) -> bool {
    for i in 0..reader.len() {
        if let Ok(file) = reader.by_index(i) {
            if file.name().starts_with("BOOT-INF/") {
                return true;
            }
        }
    }
    false
}

/// Extract classpath from Java command line arguments
fn get_class_path(args: &[String]) -> Vec<String> {
    let mut class_path = Vec::new();
    let mut next_is_cp = false;
    
    for arg in args {
        if next_is_cp {
            class_path = arg.split(':').map(|s| s.to_string()).collect();
            break;
        }
        
        if arg == "-cp" || arg == "-classpath" {
            next_is_cp = true;
        } else if let Some(cp_value) = arg.strip_prefix("-cp=").or_else(|| arg.strip_prefix("-classpath=")) {
            class_path = cp_value.split(':').map(|s| s.to_string()).collect();
            break;
        }
    }
    
    if class_path.is_empty() {
        // Default classpath is current directory
        class_path.push(".".to_string());
    }
    
    class_path
}

#[cfg(test)]
mod tests {
    use super::*;
    
    #[test]
    fn test_extract_spring_application_name() {
        let mut props = HashMap::new();
        props.insert("spring.application.name".to_string(), "my-spring-app".to_string());
        
        let result = extract_spring_application_name(&props);
        assert_eq!(result, Some("my-spring-app".to_string()));
    }
    
    #[test]
    fn test_is_spring_boot_application() {
        assert!(is_spring_boot_application("org.springframework.boot.loader.JarLauncher"));
        assert!(is_spring_boot_application("com.example.MyApplication"));
        assert!(!is_spring_boot_application("com.example.Main"));
    }
}