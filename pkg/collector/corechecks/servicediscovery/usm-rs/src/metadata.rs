use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Source of a detected service name
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum ServiceNameSource {
    /// Name derived from command line arguments
    CommandLine,
    /// Laravel application name
    Laravel,
    /// Python package/module name
    Python,
    /// Node.js package name from package.json
    Nodejs,
    /// Gunicorn application name
    Gunicorn,
    /// Rails application name
    Rails,
    /// Spring Boot application name
    Spring,
    /// JBoss application name
    JBoss,
    /// Tomcat application name
    Tomcat,
    /// WebLogic application name
    WebLogic,
    /// WebSphere application name
    WebSphere,
    /// Django application name
    Django,
    /// Flask application name
    Flask,
    /// FastAPI application name
    FastAPI,
    /// ASP.NET application name
    AspNet,
}

impl ServiceNameSource {
    pub fn as_str(&self) -> &'static str {
        match self {
            ServiceNameSource::CommandLine => "command-line",
            ServiceNameSource::Laravel => "laravel",
            ServiceNameSource::Python => "python",
            ServiceNameSource::Nodejs => "nodejs",
            ServiceNameSource::Gunicorn => "gunicorn",
            ServiceNameSource::Rails => "rails",
            ServiceNameSource::Spring => "spring",
            ServiceNameSource::JBoss => "jboss",
            ServiceNameSource::Tomcat => "tomcat",
            ServiceNameSource::WebLogic => "weblogic",
            ServiceNameSource::WebSphere => "websphere",
            ServiceNameSource::Django => "django",
            ServiceNameSource::Flask => "flask",
            ServiceNameSource::FastAPI => "fastapi",
            ServiceNameSource::AspNet => "aspnet",
        }
    }
}

impl std::fmt::Display for ServiceNameSource {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

/// Holds information about a detected service
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ServiceMetadata {
    /// Primary service name
    pub name: String,
    /// Source of the service name
    pub source: ServiceNameSource,
    /// Additional names detected (e.g., context roots)
    pub additional_names: Vec<String>,
    /// DD_SERVICE environment variable value
    pub dd_service: Option<String>,
    /// Whether DD_SERVICE was injected by auto-instrumentation
    pub dd_service_injected: bool,
    /// Additional metadata for future use
    pub metadata: HashMap<String, String>,
}

impl ServiceMetadata {
    /// Create new service metadata with required fields
    pub fn new(name: String, source: ServiceNameSource) -> Self {
        Self {
            name,
            source,
            additional_names: Vec::new(),
            dd_service: None,
            dd_service_injected: false,
            metadata: HashMap::new(),
        }
    }

    /// Create service metadata with additional names
    pub fn with_additional(
        name: String,
        source: ServiceNameSource,
        mut additional_names: Vec<String>,
    ) -> Self {
        // Sort additional names for consistent ordering
        if additional_names.len() > 1 {
            additional_names.sort();
        }
        
        Self {
            name,
            source,
            additional_names,
            dd_service: None,
            dd_service_injected: false,
            metadata: HashMap::new(),
        }
    }

    /// Set additional names for the service
    pub fn set_additional_names(&mut self, mut additional_names: Vec<String>) {
        if additional_names.len() > 1 {
            additional_names.sort();
        }
        self.additional_names = additional_names;
    }

    /// Set all names (primary and additional) and source
    pub fn set_names(
        &mut self,
        name: String,
        source: ServiceNameSource,
        additional_names: Vec<String>,
    ) {
        self.name = name;
        self.source = source;
        self.set_additional_names(additional_names);
    }

    /// Get the service key (primary name or joined additional names)
    pub fn get_service_key(&self) -> String {
        if !self.additional_names.is_empty() {
            self.additional_names.join("_")
        } else {
            self.name.clone()
        }
    }

    /// Set DD_SERVICE value
    pub fn set_dd_service(&mut self, dd_service: Option<String>, injected: bool) {
        self.dd_service = dd_service;
        self.dd_service_injected = injected;
    }

    /// Add custom metadata
    pub fn add_metadata(&mut self, key: String, value: String) {
        self.metadata.insert(key, value);
    }

    /// Check if this metadata represents a valid service
    pub fn is_valid(&self) -> bool {
        !self.name.trim().is_empty()
    }
}

impl Default for ServiceMetadata {
    fn default() -> Self {
        Self {
            name: String::new(),
            source: ServiceNameSource::CommandLine,
            additional_names: Vec::new(),
            dd_service: None,
            dd_service_injected: false,
            metadata: HashMap::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_service_metadata_creation() {
        let metadata = ServiceMetadata::new("my-service".to_string(), ServiceNameSource::Spring);
        assert_eq!(metadata.name, "my-service");
        assert_eq!(metadata.source, ServiceNameSource::Spring);
        assert!(metadata.additional_names.is_empty());
        assert!(metadata.is_valid());
    }

    #[test]
    fn test_service_key() {
        let mut metadata = ServiceMetadata::new("primary".to_string(), ServiceNameSource::CommandLine);
        assert_eq!(metadata.get_service_key(), "primary");

        metadata.set_additional_names(vec!["app1".to_string(), "app2".to_string()]);
        assert_eq!(metadata.get_service_key(), "app1_app2");
    }

    #[test]
    fn test_additional_names_sorting() {
        let metadata = ServiceMetadata::with_additional(
            "primary".to_string(),
            ServiceNameSource::Tomcat,
            vec!["zebra".to_string(), "apple".to_string(), "banana".to_string()],
        );
        assert_eq!(metadata.additional_names, vec!["apple", "banana", "zebra"]);
    }
}