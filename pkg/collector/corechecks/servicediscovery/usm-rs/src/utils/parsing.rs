use crate::error::{UsmError, UsmResult};
use std::collections::HashMap;
use std::io::Read;

/// Parse Java-style properties from a reader
pub fn parse_properties<R: Read>(mut reader: R) -> UsmResult<HashMap<String, String>> {
    let mut content = String::new();
    reader.read_to_string(&mut content)?;
    
    let mut properties = HashMap::new();
    
    for line in content.lines() {
        let line = line.trim();
        
        // Skip empty lines and comments
        if line.is_empty() || line.starts_with('#') || line.starts_with('!') {
            continue;
        }
        
        // Find the first = or : separator
        if let Some(sep_pos) = line.find(|c| c == '=' || c == ':') {
            let key = line[..sep_pos].trim();
            let value = line[sep_pos + 1..].trim();
            
            if !key.is_empty() {
                properties.insert(key.to_string(), value.to_string());
            }
        }
    }
    
    Ok(properties)
}

/// Parse YAML content into a HashMap
#[cfg(feature = "yaml")]
pub fn parse_yaml_properties<R: Read>(reader: R) -> UsmResult<HashMap<String, String>> {
    use serde_yaml::Value;
    
    let value: Value = serde_yaml::from_reader(reader)?;
    let mut properties = HashMap::new();
    
    fn flatten_yaml(value: &Value, prefix: String, map: &mut HashMap<String, String>) {
        match value {
            Value::Mapping(mapping) => {
                for (key, val) in mapping {
                    if let Value::String(key_str) = key {
                        let new_prefix = if prefix.is_empty() {
                            key_str.clone()
                        } else {
                            format!("{}.{}", prefix, key_str)
                        };
                        flatten_yaml(val, new_prefix, map);
                    }
                }
            }
            Value::String(s) => {
                map.insert(prefix, s.clone());
            }
            Value::Number(n) => {
                map.insert(prefix, n.to_string());
            }
            Value::Bool(b) => {
                map.insert(prefix, b.to_string());
            }
            _ => {}
        }
    }
    
    flatten_yaml(&value, String::new(), &mut properties);
    Ok(properties)
}

/// Parse XML content and extract text values by tag name
#[cfg(feature = "xml")]
pub fn parse_xml_properties<R: Read>(mut reader: R) -> UsmResult<HashMap<String, String>> {
    use quick_xml::events::Event;
    use quick_xml::Reader;
    
    let mut content = String::new();
    reader.read_to_string(&mut content)?;
    
    let mut xml_reader = Reader::from_str(&content);
    xml_reader.trim_text(true);
    
    let mut properties = HashMap::new();
    let mut buf = Vec::new();
    let mut current_tag = String::new();
    
    loop {
        match xml_reader.read_event_into(&mut buf) {
            Ok(Event::Start(ref e)) => {
                current_tag = String::from_utf8_lossy(e.name().as_ref()).to_string();
            }
            Ok(Event::Text(e)) => {
                if !current_tag.is_empty() {
                    let text = e.unescape().unwrap_or_default();
                    if !text.trim().is_empty() {
                        properties.insert(current_tag.clone(), text.to_string());
                    }
                }
            }
            Ok(Event::End(_)) => {
                current_tag.clear();
            }
            Ok(Event::Eof) => break,
            Err(e) => return Err(UsmError::Parse(format!("XML parse error: {}", e))),
            _ => {}
        }
        buf.clear();
    }
    
    Ok(properties)
}

/// Parse JSON content into a flat HashMap
pub fn parse_json_properties<R: Read>(reader: R) -> UsmResult<HashMap<String, String>> {
    let value: serde_json::Value = serde_json::from_reader(reader)?;
    let mut properties = HashMap::new();
    
    fn flatten_json(value: &serde_json::Value, prefix: String, map: &mut HashMap<String, String>) {
        match value {
            serde_json::Value::Object(obj) => {
                for (key, val) in obj {
                    let new_prefix = if prefix.is_empty() {
                        key.clone()
                    } else {
                        format!("{}.{}", prefix, key)
                    };
                    flatten_json(val, new_prefix, map);
                }
            }
            serde_json::Value::String(s) => {
                map.insert(prefix, s.clone());
            }
            serde_json::Value::Number(n) => {
                map.insert(prefix, n.to_string());
            }
            serde_json::Value::Bool(b) => {
                map.insert(prefix, b.to_string());
            }
            _ => {}
        }
    }
    
    flatten_json(&value, String::new(), &mut properties);
    Ok(properties)
}

/// Parse command line arguments into key-value pairs and positional args
pub fn parse_command_args(args: &[String]) -> (HashMap<String, String>, Vec<String>) {
    let mut flags = HashMap::new();
    let mut positional = Vec::new();
    let mut i = 0;
    
    while i < args.len() {
        let arg = &args[i];
        
        if arg.starts_with("--") {
            // Long option
            if let Some(eq_pos) = arg.find('=') {
                // --key=value
                let key = arg[2..eq_pos].to_string();
                let value = arg[eq_pos + 1..].to_string();
                flags.insert(key, value);
            } else {
                // --key value (or just --key)
                let key = arg[2..].to_string();
                if i + 1 < args.len() && !args[i + 1].starts_with('-') {
                    flags.insert(key, args[i + 1].clone());
                    i += 1;
                } else {
                    flags.insert(key, "true".to_string());
                }
            }
        } else if arg.starts_with('-') && arg.len() > 1 {
            // Short option(s)
            let option_chars: Vec<char> = arg[1..].chars().collect();
            
            for (idx, &opt_char) in option_chars.iter().enumerate() {
                let key = opt_char.to_string();
                
                // Check if there's a value attached
                if idx == option_chars.len() - 1 && i + 1 < args.len() && !args[i + 1].starts_with('-') {
                    flags.insert(key, args[i + 1].clone());
                    i += 1;
                    break;
                } else {
                    flags.insert(key, "true".to_string());
                }
            }
        } else {
            // Positional argument
            positional.push(arg.clone());
        }
        
        i += 1;
    }
    
    (flags, positional)
}

/// Extract system properties from Java command line arguments with precedence handling
pub fn extract_java_system_properties(args: &[String]) -> HashMap<String, String> {
    let mut properties = HashMap::new();
    
    // Process in order (later arguments override earlier ones)
    for arg in args {
        if let Some(prop_value) = arg.strip_prefix("-D") {
            if let Some(eq_pos) = prop_value.find('=') {
                let key = &prop_value[..eq_pos];
                let value = &prop_value[eq_pos + 1..];
                properties.insert(key.to_string(), value.to_string());
            } else {
                properties.insert(prop_value.to_string(), "true".to_string());
            }
        }
    }
    
    properties
}

/// Create a comprehensive property resolver with full precedence handling
/// Order: command line args > system props > environment > config files
pub fn create_property_resolver(args: &[String], env_vars: &HashMap<String, String>) -> PropertyResolver {
    let mut resolver = PropertyResolver::new();
    
    // 1. Lowest priority: config files (would be added by specific detectors)
    // Config files are handled by individual framework parsers
    
    // 2. Environment variables
    resolver.add_env_properties(env_vars);
    
    // 3. Java system properties (-D)
    resolver.add_system_properties(&extract_java_system_properties(args));
    
    // 4. Highest priority: command line arguments (--key=value)
    let (flags, _) = parse_command_args(args);
    resolver.add_command_line_properties(&flags);
    
    resolver
}

/// Multi-source property resolver with precedence handling
#[derive(Debug, Default)]
pub struct PropertyResolver {
    sources: Vec<PropertySource>,
}

#[derive(Debug)]
struct PropertySource {
    name: String,
    properties: HashMap<String, String>,
    priority: u8, // Higher number = higher priority
}

impl PropertyResolver {
    pub fn new() -> Self {
        Self {
            sources: Vec::new(),
        }
    }
    
    /// Add environment variables as property source
    pub fn add_env_properties(&mut self, env_vars: &HashMap<String, String>) {
        let mut env_props = HashMap::new();
        
        // Map common environment patterns to property keys
        for (key, value) in env_vars {
            // Convert environment variable names to property format
            // SPRING_APPLICATION_NAME -> spring.application.name
            let prop_key = key.to_lowercase().replace('_', ".");
            env_props.insert(prop_key, value.clone());
            
            // Also keep original key for direct lookup
            env_props.insert(key.clone(), value.clone());
        }
        
        self.sources.push(PropertySource {
            name: "environment".to_string(),
            properties: env_props,
            priority: 2,
        });
    }
    
    /// Add Java system properties as property source
    pub fn add_system_properties(&mut self, sys_props: &HashMap<String, String>) {
        self.sources.push(PropertySource {
            name: "system".to_string(),
            properties: sys_props.clone(),
            priority: 3,
        });
    }
    
    /// Add command line properties as property source
    pub fn add_command_line_properties(&mut self, cmd_props: &HashMap<String, String>) {
        self.sources.push(PropertySource {
            name: "command_line".to_string(),
            properties: cmd_props.clone(),
            priority: 4,
        });
    }
    
    /// Add config file properties as property source
    pub fn add_config_properties(&mut self, config_name: &str, config_props: HashMap<String, String>) {
        self.sources.push(PropertySource {
            name: config_name.to_string(),
            properties: config_props,
            priority: 1,
        });
    }
    
    /// Get property value with precedence resolution
    pub fn get_property(&self, key: &str) -> Option<String> {
        // Sort sources by priority (highest first)
        let mut sorted_sources = self.sources.iter().collect::<Vec<_>>();
        sorted_sources.sort_by(|a, b| b.priority.cmp(&a.priority));
        
        for source in sorted_sources {
            if let Some(value) = source.properties.get(key) {
                return Some(value.clone());
            }
        }
        
        None
    }
    
    /// Get property with default value
    pub fn get_property_with_default(&self, key: &str, default: &str) -> String {
        self.get_property(key).unwrap_or_else(|| default.to_string())
    }
    
    /// Get all properties merged with precedence
    pub fn get_all_properties(&self) -> HashMap<String, String> {
        let mut result = HashMap::new();
        
        // Sort sources by priority (lowest first, so higher priority overwrites)
        let mut sorted_sources = self.sources.iter().collect::<Vec<_>>();
        sorted_sources.sort_by(|a, b| a.priority.cmp(&b.priority));
        
        for source in sorted_sources {
            result.extend(source.properties.iter().map(|(k, v)| (k.clone(), v.clone())));
        }
        
        result
    }
    
    /// Resolve placeholders in property values
    pub fn resolve_placeholders(&self, value: &str) -> String {
        let mut result = value.to_string();
        let mut changed = true;
        let mut iterations = 0;
        const MAX_ITERATIONS: usize = 10;
        
        while changed && iterations < MAX_ITERATIONS {
            changed = false;
            iterations += 1;
            
            // Find ${key} or ${key:default} patterns
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
                    if let Some(resolved_value) = self.get_property(key) {
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    #[test]
    fn test_parse_properties() {
        let content = r#"
# This is a comment
! This is also a comment
key1=value1
key2 = value with spaces
key3:another value
empty.key=

invalid line without separator
"#;
        
        let cursor = Cursor::new(content);
        let properties = parse_properties(cursor).unwrap();
        
        assert_eq!(properties.get("key1"), Some(&"value1".to_string()));
        assert_eq!(properties.get("key2"), Some(&"value with spaces".to_string()));
        assert_eq!(properties.get("key3"), Some(&"another value".to_string()));
        assert_eq!(properties.get("empty.key"), Some(&"".to_string()));
        assert!(!properties.contains_key("invalid"));
    }

    #[test]
    fn test_parse_json_properties() {
        let content = r#"
{
    "name": "my-app",
    "version": "1.0.0",
    "config": {
        "database": "postgres",
        "port": 8080
    }
}
"#;
        
        let cursor = Cursor::new(content);
        let properties = parse_json_properties(cursor).unwrap();
        
        assert_eq!(properties.get("name"), Some(&"my-app".to_string()));
        assert_eq!(properties.get("version"), Some(&"1.0.0".to_string()));
        assert_eq!(properties.get("config.database"), Some(&"postgres".to_string()));
        assert_eq!(properties.get("config.port"), Some(&"8080".to_string()));
    }

    #[test]
    fn test_parse_command_args() {
        let args = vec![
            "--verbose".to_string(),
            "--config=app.properties".to_string(),
            "-jar".to_string(),
            "app.jar".to_string(),
            "--port".to_string(),
            "8080".to_string(),
            "positional".to_string(),
        ];
        
        let (flags, positional) = parse_command_args(&args);
        
        assert_eq!(flags.get("verbose"), Some(&"true".to_string()));
        assert_eq!(flags.get("config"), Some(&"app.properties".to_string()));
        assert_eq!(flags.get("r"), Some(&"app.jar".to_string()));
        assert_eq!(flags.get("port"), Some(&"8080".to_string()));
        assert_eq!(positional, vec!["positional".to_string()]);
    }

    #[test]
    fn test_extract_java_system_properties() {
        let args = vec![
            "-Dspring.application.name=my-app".to_string(),
            "-Dserver.port=8080".to_string(),
            "-Ddebug".to_string(),
            "-jar".to_string(),
            "app.jar".to_string(),
        ];
        
        let properties = extract_java_system_properties(&args);
        
        assert_eq!(properties.get("spring.application.name"), Some(&"my-app".to_string()));
        assert_eq!(properties.get("server.port"), Some(&"8080".to_string()));
        assert_eq!(properties.get("debug"), Some(&"true".to_string()));
        assert_eq!(properties.len(), 3);
    }
}