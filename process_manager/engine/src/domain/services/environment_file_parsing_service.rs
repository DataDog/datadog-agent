//! Environment File Parsing Service
//!
//! Domain service that handles parsing environment variable files.
//! Supports standard KEY=VALUE format with comments and quoted values.

use crate::domain::DomainError;
use std::collections::HashMap;

/// Environment File Parsing Service
///
/// Parses environment variable files in KEY=VALUE format.
/// Supports:
/// - Comments (lines starting with #)
/// - Empty lines (ignored)
/// - Quoted values (single or double quotes)
/// - Whitespace trimming
pub struct EnvironmentFileParsingService;

impl EnvironmentFileParsingService {
    /// Parse an environment file from disk
    ///
    /// Reads the file and parses its contents into a HashMap of environment variables.
    pub fn parse_file(path: &str) -> Result<HashMap<String, String>, DomainError> {
        let contents = std::fs::read_to_string(path).map_err(|e| {
            DomainError::InvalidCommand(format!(
                "Failed to read environment file '{}': {}",
                path, e
            ))
        })?;

        Self::parse_content(&contents)
    }

    /// Parse environment file content from a string
    ///
    /// This method is separated from file I/O to enable testing without filesystem.
    /// Parses KEY=VALUE format with support for:
    /// - Comments (# at start of line)
    /// - Empty lines
    /// - Quoted values ("value" or 'value')
    /// - Whitespace trimming
    pub fn parse_content(content: &str) -> Result<HashMap<String, String>, DomainError> {
        let mut env_vars = HashMap::new();

        for (line_num, line) in content.lines().enumerate() {
            let line = line.trim();

            // Skip empty lines and comments
            if line.is_empty() || line.starts_with('#') {
                continue;
            }

            // Parse KEY=VALUE format
            if let Some((key, value)) = line.split_once('=') {
                let key = key.trim();
                let value = value.trim();

                // Validate key is not empty
                if key.is_empty() {
                    return Err(DomainError::InvalidCommand(format!(
                        "Invalid environment file format at line {}: key cannot be empty",
                        line_num + 1
                    )));
                }

                // Remove quotes if present
                let value = Self::unquote_value(value);

                env_vars.insert(key.to_string(), value);
            } else {
                return Err(DomainError::InvalidCommand(format!(
                    "Invalid environment file format at line {}: expected KEY=VALUE format, got '{}'",
                    line_num + 1,
                    line
                )));
            }
        }

        Ok(env_vars)
    }

    /// Remove surrounding quotes from a value if present
    ///
    /// Supports both single quotes ('value') and double quotes ("value").
    fn unquote_value(value: &str) -> String {
        if (value.starts_with('"') && value.ends_with('"'))
            || (value.starts_with('\'') && value.ends_with('\''))
        {
            if value.len() >= 2 {
                value[1..value.len() - 1].to_string()
            } else {
                String::new()
            }
        } else {
            value.to_string()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_content_basic() {
        let content = "KEY1=value1\nKEY2=value2";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.get("KEY1"), Some(&"value1".to_string()));
        assert_eq!(result.get("KEY2"), Some(&"value2".to_string()));
    }

    #[test]
    fn test_parse_content_with_comments() {
        let content = "# This is a comment\nKEY1=value1\n# Another comment\nKEY2=value2";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.len(), 2);
        assert_eq!(result.get("KEY1"), Some(&"value1".to_string()));
        assert_eq!(result.get("KEY2"), Some(&"value2".to_string()));
    }

    #[test]
    fn test_parse_content_with_empty_lines() {
        let content = "KEY1=value1\n\n\nKEY2=value2\n\n";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.len(), 2);
        assert_eq!(result.get("KEY1"), Some(&"value1".to_string()));
        assert_eq!(result.get("KEY2"), Some(&"value2".to_string()));
    }

    #[test]
    fn test_parse_content_with_double_quotes() {
        let content = r#"KEY1="value with spaces""#;
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.get("KEY1"), Some(&"value with spaces".to_string()));
    }

    #[test]
    fn test_parse_content_with_single_quotes() {
        let content = "KEY1='value with spaces'";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.get("KEY1"), Some(&"value with spaces".to_string()));
    }

    #[test]
    fn test_parse_content_with_whitespace() {
        let content = "  KEY1  =  value1  \n  KEY2=value2";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.get("KEY1"), Some(&"value1".to_string()));
        assert_eq!(result.get("KEY2"), Some(&"value2".to_string()));
    }

    #[test]
    fn test_parse_content_empty_value() {
        let content = "KEY1=";
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.get("KEY1"), Some(&"".to_string()));
    }

    #[test]
    fn test_parse_content_invalid_format_no_equals() {
        let content = "INVALID_LINE";
        let result = EnvironmentFileParsingService::parse_content(content);

        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("expected KEY=VALUE format"));
    }

    #[test]
    fn test_parse_content_invalid_empty_key() {
        let content = "=value";
        let result = EnvironmentFileParsingService::parse_content(content);

        assert!(matches!(result, Err(DomainError::InvalidCommand(_))));
        assert!(result
            .unwrap_err()
            .to_string()
            .contains("key cannot be empty"));
    }

    #[test]
    fn test_parse_content_complex_example() {
        let content = r#"
# Database configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME="my_database"

# API configuration
API_KEY='secret-key-123'
API_TIMEOUT=30

# Empty value
OPTIONAL_FIELD=
"#;
        let result = EnvironmentFileParsingService::parse_content(content).unwrap();

        assert_eq!(result.len(), 6);
        assert_eq!(result.get("DB_HOST"), Some(&"localhost".to_string()));
        assert_eq!(result.get("DB_PORT"), Some(&"5432".to_string()));
        assert_eq!(result.get("DB_NAME"), Some(&"my_database".to_string()));
        assert_eq!(result.get("API_KEY"), Some(&"secret-key-123".to_string()));
        assert_eq!(result.get("API_TIMEOUT"), Some(&"30".to_string()));
        assert_eq!(result.get("OPTIONAL_FIELD"), Some(&"".to_string()));
    }

    #[test]
    fn test_unquote_value_double_quotes() {
        let value = EnvironmentFileParsingService::unquote_value("\"hello\"");
        assert_eq!(value, "hello");
    }

    #[test]
    fn test_unquote_value_single_quotes() {
        let value = EnvironmentFileParsingService::unquote_value("'hello'");
        assert_eq!(value, "hello");
    }

    #[test]
    fn test_unquote_value_no_quotes() {
        let value = EnvironmentFileParsingService::unquote_value("hello");
        assert_eq!(value, "hello");
    }

    #[test]
    fn test_unquote_value_empty_quotes() {
        let value = EnvironmentFileParsingService::unquote_value("\"\"");
        assert_eq!(value, "");
    }

    #[test]
    fn test_unquote_value_mismatched_quotes() {
        let value = EnvironmentFileParsingService::unquote_value("\"hello'");
        assert_eq!(value, "\"hello'"); // Not unquoted if mismatched
    }
}
