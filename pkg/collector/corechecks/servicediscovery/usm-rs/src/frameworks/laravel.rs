// Laravel detection framework

use crate::context::DetectionContext;
use std::io::{BufRead, BufReader, Read};
use std::path::Path;
use regex::Regex;

/// Laravel parser for extracting application names from Laravel projects
pub struct LaravelParser<'a> {
    ctx: &'a mut DetectionContext,
}

impl<'a> LaravelParser<'a> {
    pub fn new(ctx: &'a mut DetectionContext) -> Self {
        Self { ctx }
    }
    
    /// Get Laravel application name from artisan command
    pub fn get_laravel_app_name(&mut self, artisan: &str) -> String {
        let laravel_dir = Path::new(artisan).parent().unwrap_or(Path::new(".")).to_string_lossy();
        
        // Try to get name from .env file first
        if let Some(name) = self.get_laravel_app_name_from_env(&laravel_dir) {
            return name;
        }
        
        // Try to get name from config/app.php
        if let Some(name) = self.get_laravel_app_name_from_config(&laravel_dir) {
            return name;
        }
        
        // Default fallback
        "laravel".to_string()
    }
    
    /// Extract application name from .env file
    fn get_laravel_app_name_from_env(&self, laravel_dir: &str) -> Option<String> {
        let env_file_path = format!("{}/.env", laravel_dir);
        
        if let Some(name) = self.trim_prefix_from_line(&env_file_path, "DD_SERVICE=") {
            return Some(name);
        }
        
        if let Some(name) = self.trim_prefix_from_line(&env_file_path, "OTEL_SERVICE_NAME=") {
            return Some(name);
        }
        
        if let Some(name) = self.trim_prefix_from_line(&env_file_path, "APP_NAME=") {
            return Some(name);
        }
        
        None
    }
    
    /// Extract application name from config/app.php
    fn get_laravel_app_name_from_config(&self, laravel_dir: &str) -> Option<String> {
        let config_file_path = format!("{}/config/app.php", laravel_dir);
        
        if let Ok(file) = self.ctx.filesystem.open(std::path::Path::new(&config_file_path)) {
            let mut content = String::new();
            let mut reader = BufReader::new(file);
            
            if reader.read_to_string(&mut content).is_ok() {
                // Try to match Laravel config patterns
                return self.get_first_match_from_regex(
                    r#"env\(\s*["']APP_NAME["']\s*,\s*["'](.*?)["']\s*\)|["']name["']\s*=>\s*["'](.*?)["']"#,
                    content.as_bytes()
                );
            }
        }
        
        None
    }
    
    /// Extract first line that starts with the given prefix from a file
    fn trim_prefix_from_line(&self, file_path: &str, prefix: &str) -> Option<String> {
        if let Ok(file) = self.ctx.filesystem.open(std::path::Path::new(file_path)) {
            let reader = BufReader::new(file);
            
            for line in reader.lines() {
                if let Ok(line_str) = line {
                    if let Some(value) = line_str.strip_prefix(prefix) {
                        let trimmed = value.trim();
                        // Remove quotes if present
                        let cleaned = trimmed.trim_matches('"').trim_matches('\'');
                        if !cleaned.is_empty() {
                            return Some(cleaned.to_string());
                        }
                    }
                }
            }
        }
        
        None
    }
    
    /// Get first match from regex pattern
    fn get_first_match_from_regex(&self, pattern: &str, content: &[u8]) -> Option<String> {
        if let Ok(regex) = Regex::new(pattern) {
            if let Ok(content_str) = std::str::from_utf8(content) {
                if let Some(captures) = regex.captures(content_str) {
                    // Try capture groups 1 and 2 (the parentheses in the pattern)
                    for i in 1..=2 {
                        if let Some(matched) = captures.get(i) {
                            let value = matched.as_str();
                            if !value.is_empty() {
                                return Some(value.to_string());
                            }
                        }
                    }
                }
            }
        }
        
        None
    }
}

/// Extract Laravel application name from configuration
pub fn extract_laravel_app_name(config: &std::collections::HashMap<String, String>) -> Option<String> {
    // Check common Laravel app name keys
    let app_keys = [
        "DD_SERVICE",
        "OTEL_SERVICE_NAME", 
        "APP_NAME",
        "LARAVEL_APP_NAME",
    ];
    
    for key in &app_keys {
        if let Some(name) = config.get(*key) {
            if !name.trim().is_empty() {
                return Some(name.trim().to_string());
            }
        }
    }
    
    None
}

/// Check if this appears to be a Laravel application
pub fn is_laravel_application(path: &str) -> bool {
    // Check for Laravel-specific files/directories
    path.contains("artisan") || 
    path.contains("app/Http/Kernel.php") ||
    path.contains("bootstrap/app.php") ||
    path.contains("config/app.php")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::context::Environment;
    use crate::filesystem::MemoryFileSystem;
    use std::sync::Arc;
    use std::collections::HashMap;
    
    
    #[test]
    fn test_is_laravel_application() {
        assert!(is_laravel_application("/var/www/myapp/artisan"));
        assert!(is_laravel_application("/app/bootstrap/app.php"));
        assert!(is_laravel_application("/app/config/app.php"));
        assert!(!is_laravel_application("/usr/bin/php"));
    }
    
    #[test]
    fn test_extract_laravel_app_name() {
        let mut config = HashMap::new();
        config.insert("APP_NAME".to_string(), "MyLaravelApp".to_string());
        
        let result = extract_laravel_app_name(&config);
        assert_eq!(result, Some("MyLaravelApp".to_string()));
        
        let mut config2 = HashMap::new();
        config2.insert("DD_SERVICE".to_string(), "custom-service".to_string());
        
        let result2 = extract_laravel_app_name(&config2);
        assert_eq!(result2, Some("custom-service".to_string()));
    }
    
    #[test]
    fn test_laravel_parser_from_env() {
        let mut fs = MemoryFileSystem::new();
        fs.add_file("./.env", b"DD_SERVICE=my-laravel-service\nAPP_NAME=MyApp\n".to_vec());
        
        let filesystem = Arc::new(fs);
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, filesystem);
        
        let mut parser = LaravelParser::new(&mut ctx);
        let result = parser.get_laravel_app_name("./artisan");
        
        assert_eq!(result, "my-laravel-service");
    }
    
    #[test]
    fn test_laravel_parser_fallback() {
        let fs = Arc::new(MemoryFileSystem::new());
        let env = Environment::new();
        let mut ctx = DetectionContext::new(vec![], env, fs);
        
        let mut parser = LaravelParser::new(&mut ctx);
        let result = parser.get_laravel_app_name("artisan");
        
        assert_eq!(result, "laravel");
    }
}