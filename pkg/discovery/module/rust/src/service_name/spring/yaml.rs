// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::io::Read;
use yaml_rust2::{Yaml, YamlLoader};

/// Parser for YAML that searches for a specific string property in dot
/// notation.
pub fn parse_yaml<R: Read>(mut reader: R, target_key: &str) -> Option<String> {
    // Read the YAML content
    let mut content = String::new();
    reader.read_to_string(&mut content).ok()?;

    // Parse YAML. This could use a streaming parser, but since that is quite
    // complex, just load the entire document since it should be small (And the
    // size has been restricted by the caller).
    let docs = YamlLoader::load_from_str(&content).ok()?;
    let doc = docs.first()?;

    // Split target key into path components and navigate
    let mut current = doc;
    for key in target_key.split('.') {
        current = &current[key];
        if current.is_badvalue() {
            return None;
        }
    }

    match current {
        Yaml::String(s) => Some(s.clone()),
        _ => None,
    }
}
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_yaml_streaming() {
        let yaml = r#"
spring:
  application:
    name: myapp
partial:
  existent: 1
server:
  port: 8080
other:
  deeply:
    nested:
      value: test
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("myapp".to_string()));

        // Test property not found
        let result2 = parse_yaml(yaml, "non.existent.key");
        assert_eq!(result2, None);
        let result2 = parse_yaml(yaml, "partial.existent.key");
        assert_eq!(result2, None);

        // Test deeply nested property
        let result3 = parse_yaml(yaml, "other.deeply.nested.value");
        assert_eq!(result3, Some("test".to_string()));
    }

    #[test]
    fn test_parse_yaml_invalid_types() {
        let yaml = r#"
spring:
  application:
    name: myapp
    port: 8080
    enabled: true
    tags:
      - tag1
      - tag2
    config:
      timeout: 30
"#
        .as_bytes();

        // Test integer value returns None
        let result = parse_yaml(yaml, "spring.application.port");
        assert_eq!(result, None);

        // Test boolean value returns None
        let result = parse_yaml(yaml, "spring.application.enabled");
        assert_eq!(result, None);

        // Test array value returns None
        let result = parse_yaml(yaml, "spring.application.tags");
        assert_eq!(result, None);

        // Test hash/object value returns None
        let result = parse_yaml(yaml, "spring.application.config");
        assert_eq!(result, None);
    }

    #[test]
    fn test_parse_yaml_edge_cases() {
        // Test empty YAML
        let yaml = "".as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test YAML with only whitespace
        let yaml = "   \n  \n  ".as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test malformed YAML
        let yaml = r#"
spring:
  application:
    name: myapp
  invalid indentation
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);

        // Test empty string value
        let yaml = r#"
spring:
  application:
    name: ""
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, Some("".to_string()));

        // Test null value
        let yaml = r#"
spring:
  application:
    name: null
"#
        .as_bytes();
        let result = parse_yaml(yaml, "spring.application.name");
        assert_eq!(result, None);
    }
}
