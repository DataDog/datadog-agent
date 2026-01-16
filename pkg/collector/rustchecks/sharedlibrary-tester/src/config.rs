use std::fs;

use anyhow::{Ok, Result};
use serde_yaml::{Mapping, Value};

pub struct Config {
    pub init_config: String,
    pub instances: Vec<String>,
}

impl Config {
    pub fn extract(path: &str) -> Result<Config> {
        let content = fs::read_to_string(path)?;
        let yaml: Value = serde_yaml::from_str(&content)?;

        let init_config = match yaml.get("init_config") {
            Some(Value::Mapping(map)) => mapping_to_yaml_string(map),
            Some(Value::Null) | None => String::new(),
            Some(other) => serde_yaml::to_string(other)?.trim_start_matches("---\n").to_string(),
        };

        let instances = match yaml.get("instances") {
            Some(Value::Sequence(seq)) => {
                seq.iter()
                    .map(|instance| {
                        match instance {
                            Value::Mapping(map) => mapping_to_yaml_string(map),
                            Value::Null => String::new(),
                            other => serde_yaml::to_string(other)
                                .unwrap_or_default()
                                .trim_start_matches("---\n")
                                .to_string(),
                        }
                    })
                    .collect()
            }
            Some(Value::Null) | None => vec![],
            _ => vec![],
        };

        Ok(Config { init_config, instances })
    }
}

/// Convert a YAML mapping to a string without the document separator
fn mapping_to_yaml_string(map: &Mapping) -> String {
    if map.is_empty() {
        return String::new();
    }
    
    let lines: Vec<String> = map
        .iter()
        .filter_map(|(k, v)| {
            let key = match k {
                Value::String(s) => s.clone(),
                other => serde_yaml::to_string(other).ok()?.trim().to_string(),
            };
            
            let value = match v {
                Value::String(s) => s.clone(),
                Value::Number(n) => n.to_string(),
                Value::Bool(b) => b.to_string(),
                Value::Null => "".to_string(),
                // For complex values (maps, sequences), serialize them
                other => {
                    let serialized = serde_yaml::to_string(other).ok()?;
                    return Some(format!("{}:\n{}", key, indent_string(&serialized.trim_start_matches("---\n"), 2)));
                }
            };
            
            Some(format!("{}: {}", key, value))
        })
        .collect();
    
    if lines.is_empty() {
        String::new()
    } else {
        lines.join("\n") + "\n"
    }
}

/// Indent each line of a string by the specified number of spaces
fn indent_string(s: &str, spaces: usize) -> String {
    let indent = " ".repeat(spaces);
    s.lines()
        .map(|line| format!("{}{}", indent, line))
        .collect::<Vec<_>>()
        .join("\n")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_configs() {
        let yaml = r#"
init_config:
  timeout: 30

instances:
  - host: localhost
    port: 8080
  - host: remotehost
    port: 9090
"#;
        
        // Write to a temp file
        let temp_path = "/tmp/test_config.yaml";
        fs::write(temp_path, yaml).unwrap();
        
        let config = Config::extract(temp_path).unwrap();
        
        assert_eq!(config.init_config, "timeout: 30\n");
        assert_eq!(config.instances.len(), 2);
        assert_eq!(config.instances[0], "host: localhost\nport: 8080\n");
        assert_eq!(config.instances[1], "host: remotehost\nport: 9090\n");
        
        // Cleanup
        fs::remove_file(temp_path).ok();
    }

    #[test]
    fn test_empty_instance() {
        let yaml = r#"
init_config:

instances:
  -
"#;
        let temp_path = "/tmp/test_empty_instance.yaml";
        fs::write(temp_path, yaml).unwrap();
        
        let config = Config::extract(temp_path).unwrap();
        
        assert_eq!(config.init_config, "");
        assert_eq!(config.instances.len(), 1);
        assert_eq!(config.instances[0], "");
        
        fs::remove_file(temp_path).ok();
    }

    #[test]
    fn test_empty_yaml() {
        let yaml = "";
        let temp_path = "/tmp/test_empty.yaml";
        fs::write(temp_path, yaml).unwrap();
        
        let config = Config::extract(temp_path).unwrap();
        
        assert_eq!(config.init_config, "");
        assert_eq!(config.instances.len(), 0);
        
        fs::remove_file(temp_path).ok();
    }

    #[test]
    fn test_null_instances() {
        let yaml = r#"
init_config:
instances:
"#;
        let temp_path = "/tmp/test_null_instances.yaml";
        fs::write(temp_path, yaml).unwrap();
        
        let config = Config::extract(temp_path).unwrap();
        
        assert_eq!(config.init_config, "");
        assert_eq!(config.instances.len(), 0);
        
        fs::remove_file(temp_path).ok();
    }
}
