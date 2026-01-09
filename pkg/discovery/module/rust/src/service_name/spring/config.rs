// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;

/// Spring Boot configuration extracted from command line arguments and environment variables
#[derive(Default)]
pub struct SpringBootConfig {
    pub app_name: Option<String>,
    pub config_location: Option<String>,
    pub config_name: Option<String>,
    pub active_profiles: Option<String>,
}

impl SpringBootConfig {
    fn assign(&mut self, key: &str, value: &str) {
        match key {
            "spring.application.name" => {
                self.app_name = Some(value.to_string());
            }
            "spring.config.locations" => {
                self.config_location = Some(value.to_string());
            }
            "spring.config.name" => {
                self.config_name = Some(value.to_string());
            }
            "spring.profiles.active" => {
                self.active_profiles = Some(value.to_string());
            }
            _ => {}
        }
    }
}

/// Extract Spring Boot configuration from command line arguments and environment variables.
/// This function searches for specific Spring Boot properties with priority order:
/// 1. Spring Boot arguments (--key=value)
/// 2. System properties (-Dkey=value)
/// 3. Environment variables
pub fn extract_spring_boot_config(
    args: impl Iterator<Item = impl AsRef<str>>,
    envs: &HashMap<String, String>,
) -> SpringBootConfig {
    let mut spring_args = SpringBootConfig::default();
    let mut system_props = SpringBootConfig::default();

    for arg in args {
        let arg = arg.as_ref();

        let mut system = false;
        let keyval = if let Some(keyval) = arg.strip_prefix("--") {
            keyval
        } else if let Some(keyval) = arg.strip_prefix("-D") {
            system = true;
            keyval
        } else {
            continue;
        };

        let Some((key, value)) = keyval.split_once('=') else {
            continue;
        };

        if system {
            system_props.assign(key, value);
        } else {
            spring_args.assign(key, value);
        }
    }

    // Apply priority: spring args > system props > env vars
    let app_name = spring_args
        .app_name
        .or(system_props.app_name)
        .or_else(|| envs.get("SPRING_APPLICATION_NAME").cloned());

    // If the app name is found, we don't need anything else
    if let Some(app_name) = app_name {
        return SpringBootConfig {
            app_name: Some(app_name),
            ..SpringBootConfig::default()
        };
    }

    let config_location = spring_args
        .config_location
        .or(system_props.config_location)
        .or_else(|| envs.get("SPRING_CONFIG_LOCATIONS").cloned());
    let config_name = spring_args
        .config_name
        .or(system_props.config_name)
        .or_else(|| envs.get("SPRING_CONFIG_NAME").cloned());
    let active_profiles = spring_args
        .active_profiles
        .or(system_props.active_profiles)
        .or_else(|| envs.get("SPRING_PROFILES_ACTIVE").cloned());

    SpringBootConfig {
        app_name,
        config_location,
        config_name,
        active_profiles,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_spring_boot_config_argument_parsing() {
        let args = [
            "-c",
            "--spring.application.name",
            "-Xspring.application.name=fake",
            "--spring.profiles.active=prod",
            "-Dspring.config.name=myapp",
        ];

        let envs = HashMap::new();
        let config = extract_spring_boot_config(args.iter().copied(), &envs);

        // Test Spring Boot arguments (--key=value)
        assert_eq!(config.active_profiles.as_deref(), Some("prod"));
        assert_eq!(config.config_name.as_deref(), Some("myapp"));
    }

    #[test]
    fn test_extract_spring_boot_config_environment_variables() {
        let mut envs = HashMap::new();
        envs.insert(
            "SPRING_CONFIG_LOCATIONS".to_string(),
            "file:./config/".to_string(),
        );

        let args: Vec<&str> = vec![];
        let config = extract_spring_boot_config(args.into_iter(), &envs);
        assert_eq!(config.config_location.as_deref(), Some("file:./config/"));
    }

    #[test]
    fn test_extract_spring_boot_config_priority_order() {
        // Test that spring args > system props > env vars
        let mut envs = HashMap::new();
        envs.insert(
            "SPRING_APPLICATION_NAME".to_string(),
            "from-env".to_string(),
        );

        let args = [
            "-Dspring.application.name=from-system",
            "--spring.application.name=from-spring",
        ];
        let config = extract_spring_boot_config(args.iter().copied(), &envs);

        // Spring args should win
        assert_eq!(config.app_name.as_deref(), Some("from-spring"));

        // Test system props > env vars
        let args2 = ["-Dspring.application.name=from-system"];
        let config2 = extract_spring_boot_config(args2.iter().copied(), &envs);
        assert_eq!(config2.app_name.as_deref(), Some("from-system"));
    }
}
