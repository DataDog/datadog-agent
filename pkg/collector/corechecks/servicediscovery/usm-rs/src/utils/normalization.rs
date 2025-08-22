use regex::Regex;
use once_cell::sync::Lazy;

/// Maximum length for normalized service names
const MAX_SERVICE_NAME_LENGTH: usize = 100;

/// Regex for validating service names
static SERVICE_NAME_REGEX: Lazy<Regex> = Lazy::new(|| {
    Regex::new(r"^[a-z][a-z0-9_.-]*[a-z0-9]$|^[a-z]$").unwrap()
});

/// Normalize service name according to Datadog standards
pub fn normalize_service_name(name: &str) -> String {
    if name.is_empty() {
        return name.to_string();
    }

    let mut normalized = name.to_lowercase();
    
    // Replace invalid characters with underscores
    normalized = normalized
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '.' || c == '-' || c == '_' {
                c
            } else {
                '_'
            }
        })
        .collect();
    
    // Remove leading/trailing underscores, dots, dashes
    normalized = normalized.trim_matches(|c| c == '_' || c == '.' || c == '-').to_string();
    
    // Ensure it starts with a letter
    if !normalized.is_empty() && !normalized.chars().next().unwrap().is_ascii_alphabetic() {
        normalized = format!("service_{}", normalized);
    }
    
    // Collapse multiple consecutive underscores, dots, dashes but preserve single ones
    let collapse_regex = Regex::new(r"[_.-]{2,}").unwrap();
    normalized = collapse_regex.replace_all(&normalized, "_").to_string();
    
    // Truncate if too long
    if normalized.len() > MAX_SERVICE_NAME_LENGTH {
        normalized.truncate(MAX_SERVICE_NAME_LENGTH);
        // Remove trailing separators after truncation
        normalized = normalized.trim_end_matches(|c| c == '_' || c == '.' || c == '-').to_string();
    }
    
    // Ensure we have something valid
    if normalized.is_empty() || !SERVICE_NAME_REGEX.is_match(&normalized) {
        return "service".to_string();
    }
    
    normalized
}

/// Extract package name from Java class name
pub fn extract_package_from_java_class(class_name: &str) -> Option<String> {
    if let Some(last_dot) = class_name.rfind('.') {
        let package = &class_name[..last_dot];
        if !package.is_empty() {
            return Some(package.to_string());
        }
    }
    None
}

/// Extract project name after org.apache. prefix
pub fn extract_apache_project_name(class_name: &str) -> Option<String> {
    const APACHE_PREFIX: &str = "org.apache.";
    if class_name.starts_with(APACHE_PREFIX) {
        let after_prefix = &class_name[APACHE_PREFIX.len()..];
        if let Some(dot_pos) = after_prefix.find('.') {
            return Some(after_prefix[..dot_pos].to_string());
        }
    }
    None
}

/// Parse module name from Python WSGI application string (e.g., "module:app" -> "module")
pub fn parse_wsgi_module_name(wsgi_app: &str) -> String {
    if let Some(colon_pos) = wsgi_app.find(':') {
        wsgi_app[..colon_pos].to_string()
    } else {
        wsgi_app.to_string()
    }
}

/// Clean and normalize directory/file names for service detection
pub fn normalize_path_component(component: &str) -> String {
    let normalized = component
        .chars()
        .filter(|&c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
        .collect::<String>()
        .to_lowercase();
    
    if normalized.is_empty() {
        "unknown".to_string()
    } else {
        normalized
    }
}

/// Remove common file extensions and return clean name
pub fn clean_filename_for_service(filename: &str) -> String {
    let name = if let Some(dot_pos) = filename.rfind('.') {
        &filename[..dot_pos]
    } else {
        filename
    };
    
    normalize_service_name(name)
}

/// Extract service name from Spring application properties
pub fn extract_spring_app_name(properties: &std::collections::HashMap<String, String>) -> Option<String> {
    // Try different Spring application name properties in order of preference
    let keys = [
        "spring.application.name",
        "spring.cloud.application.name", 
        "info.app.name",
        "management.info.app.name",
    ];
    
    for key in &keys {
        if let Some(name) = properties.get(*key) {
            let normalized = normalize_service_name(name);
            if !normalized.is_empty() && normalized != "service" {
                return Some(normalized);
            }
        }
    }
    
    None
}

/// Check if a name is generic and should be avoided
pub fn is_generic_name(name: &str) -> bool {
    let generic_names = [
        "app", "application", "service", "server", "main", "index", "start",
        "run", "launcher", "bootstrap", "init", "daemon", "worker", "process",
        "program", "exe", "bin", "sbin", "usr", "opt", "tmp", "var", "etc",
        "root", "home", "java", "python", "node", "php", "ruby", "dotnet",
    ];
    
    generic_names.contains(&name.to_lowercase().as_str())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    #[test]
    fn test_normalize_service_name() {
        assert_eq!(normalize_service_name("MyApp"), "myapp");
        assert_eq!(normalize_service_name("my-app"), "my-app");
        assert_eq!(normalize_service_name("my_app"), "my_app");
        assert_eq!(normalize_service_name("my.app"), "my.app");
        assert_eq!(normalize_service_name("My@App#2"), "my_app_2");
        assert_eq!(normalize_service_name("123app"), "service_123app");
        assert_eq!(normalize_service_name("_app_"), "app");
        assert_eq!(normalize_service_name(""), "");
        assert_eq!(normalize_service_name("___"), "service");
    }

    #[test]
    fn test_extract_package_from_java_class() {
        assert_eq!(
            extract_package_from_java_class("com.example.MyClass"),
            Some("com.example".to_string())
        );
        assert_eq!(
            extract_package_from_java_class("MyClass"),
            None
        );
        assert_eq!(
            extract_package_from_java_class(""),
            None
        );
    }

    #[test]
    fn test_extract_apache_project_name() {
        assert_eq!(
            extract_apache_project_name("org.apache.tomcat.Main"),
            Some("tomcat".to_string())
        );
        assert_eq!(
            extract_apache_project_name("org.apache.kafka"),
            None
        );
        assert_eq!(
            extract_apache_project_name("com.example.Main"),
            None
        );
    }

    #[test]
    fn test_parse_wsgi_module_name() {
        assert_eq!(parse_wsgi_module_name("myapp:application"), "myapp");
        assert_eq!(parse_wsgi_module_name("myapp"), "myapp");
        assert_eq!(parse_wsgi_module_name("package.module:app"), "package.module");
    }

    #[test]
    fn test_clean_filename_for_service() {
        assert_eq!(clean_filename_for_service("app.py"), "app");
        assert_eq!(clean_filename_for_service("MyService.jar"), "myservice");
        assert_eq!(clean_filename_for_service("complex-name.war"), "complex-name");
        assert_eq!(clean_filename_for_service("my@app#.js"), "my_app");
    }

    #[test]
    fn test_extract_spring_app_name() {
        let mut props = HashMap::new();
        props.insert("spring.application.name".to_string(), "my-spring-app".to_string());
        
        assert_eq!(
            extract_spring_app_name(&props),
            Some("my-spring-app".to_string())
        );

        let mut props2 = HashMap::new();
        props2.insert("info.app.name".to_string(), "Another App".to_string());
        
        assert_eq!(
            extract_spring_app_name(&props2),
            Some("another_app".to_string())
        );
    }

    #[test]
    fn test_is_generic_name() {
        assert!(is_generic_name("app"));
        assert!(is_generic_name("APP"));
        assert!(is_generic_name("service"));
        assert!(is_generic_name("main"));
        assert!(!is_generic_name("myapp"));
        assert!(!is_generic_name("user-service"));
    }
}