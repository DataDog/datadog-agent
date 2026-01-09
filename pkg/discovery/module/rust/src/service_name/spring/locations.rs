// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use crate::service_name::context::DetectionContext;
use normalize_path::NormalizePath;
use std::collections::HashMap;
use std::path::Path;

/// Info holds parsed location information
pub struct Info<'a> {
    pub file_patterns: HashMap<&'a str, Vec<String>>,
    pub classpath_patterns: HashMap<&'a str, Vec<String>>,
}

/// Helper function to add a pattern to either file_patterns or classpath_patterns
fn add_pattern<'a>(
    ctx: &DetectionContext,
    is_classpath: bool,
    path: String,
    profile: &'a str,
    info: &mut Info<'a>,
) {
    let cleaned = Path::new(&path).normalize();
    if is_classpath {
        info.classpath_patterns
            .entry(profile)
            .or_default()
            .push(cleaned.to_string_lossy().to_string());
    } else {
        let resolved = ctx
            .resolve_working_dir_relative_path(&cleaned)
            .unwrap_or(cleaned);
        info.file_patterns
            .entry(profile)
            .or_default()
            .push(resolved.to_string_lossy().to_string());
    }
}

/// Parse location URIs and generate file and classpath patterns
///
/// It parses locations (usually specified by the property locationPropName)
/// given the list of active profiles (specified by
/// activeProfilesPropName).
///
/// It returns a couple of maps each having as key the profile name ("" stands
/// for the default one) and as value the ant patterns where the properties
/// should be found.
///
/// The first map returned is the locations to be found in fs while the second
/// map contains locations on the classpath (usually inside the application jar)
pub fn parse<'a>(
    ctx: &DetectionContext,
    locations: impl IntoIterator<Item = &'a str>,
    name: &str,
    profiles: &'a [&'a str],
    boot_inf_jar_path: &str,
) -> Info<'a> {
    let mut info = Info {
        file_patterns: HashMap::new(),
        classpath_patterns: HashMap::new(),
    };

    for location in locations {
        let mut parts = location.split(':');

        let path = match parts.next_back() {
            Some(p) => p,
            None => continue,
        };

        let is_classpath = parts.next_back() == Some("classpath");
        let mut path = path.to_string();

        if is_classpath {
            path = format!("{}{}", boot_inf_jar_path, path);
        }

        // Normalize path separators
        path = path.replace('\\', "/");

        if path.ends_with('/') {
            // Directory: add all possible filenames with profiles
            let extensions = ["properties", "yaml", "yml"];

            for profile in profiles {
                let base = format!("{}{}-{}", path, name, profile);
                for ext in &extensions {
                    let full_path = format!("{}.{}", base, ext);
                    add_pattern(ctx, is_classpath, full_path, profile, &mut info);
                }
            }

            // Add default profile files
            let base = format!("{}{}", path, name);
            for ext in &extensions {
                let full_path = format!("{}.{}", base, ext);
                add_pattern(ctx, is_classpath, full_path, "", &mut info);
            }
        } else {
            // Direct file reference
            add_pattern(ctx, is_classpath, path, "", &mut info);
        }
    }

    info
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::indexing_slicing)]
mod tests {
    use super::*;
    use crate::fs::SubDirFs;
    use std::env;

    fn test_ctx() -> (HashMap<String, String>, SubDirFs) {
        let tempdir = env::temp_dir();
        let fs = SubDirFs::new(&tempdir).unwrap();
        let envs = HashMap::new();
        (envs, fs)
    }

    #[test]
    fn test_parse_spring_boot_defaults() {
        let (mut envs, fs) = test_ctx();
        envs.insert("PWD".to_string(), "/opt/somewhere/".to_string());
        let ctx = DetectionContext::new(0, envs, &fs);

        let default_locations = "optional:classpath:/;optional:classpath:/config/;optional:file:./;optional:file:./config/;optional:file:./config/*/";
        let default_config_name = "application";

        let uri_info = parse(
            &ctx,
            default_locations.split(';'),
            default_config_name,
            &[],
            "BOOT-INF/classes/",
        );

        // Check filesystem patterns
        let default_fs = &uri_info.file_patterns[""];
        assert_eq!(
            default_fs,
            &vec![
                "/opt/somewhere/application.properties".to_string(),
                "/opt/somewhere/application.yaml".to_string(),
                "/opt/somewhere/application.yml".to_string(),
                "/opt/somewhere/config/application.properties".to_string(),
                "/opt/somewhere/config/application.yaml".to_string(),
                "/opt/somewhere/config/application.yml".to_string(),
                "/opt/somewhere/config/*/application.properties".to_string(),
                "/opt/somewhere/config/*/application.yaml".to_string(),
                "/opt/somewhere/config/*/application.yml".to_string(),
            ]
        );

        // Check classpath patterns
        let default_cp = &uri_info.classpath_patterns[""];
        assert_eq!(
            default_cp,
            &vec![
                "BOOT-INF/classes/application.properties".to_string(),
                "BOOT-INF/classes/application.yaml".to_string(),
                "BOOT-INF/classes/application.yml".to_string(),
                "BOOT-INF/classes/config/application.properties".to_string(),
                "BOOT-INF/classes/config/application.yaml".to_string(),
                "BOOT-INF/classes/config/application.yml".to_string(),
            ]
        );
    }

    #[test]
    fn test_parse_with_profiles_and_custom_locations() {
        let (mut envs, fs) = test_ctx();
        envs.insert("PWD".to_string(), "/opt/somewhere/".to_string());
        let ctx = DetectionContext::new(0, envs, &fs);

        let uri_info = parse(
            &ctx,
            ["file:/opt/anotherdir/anotherfile.properties", "file:./"],
            "custom",
            &["prod"],
            "BOOT-INF/classes/",
        );

        // Check prod profile patterns
        let prod_fs = &uri_info.file_patterns["prod"];
        assert_eq!(
            prod_fs,
            &vec![
                "/opt/somewhere/custom-prod.properties".to_string(),
                "/opt/somewhere/custom-prod.yaml".to_string(),
                "/opt/somewhere/custom-prod.yml".to_string(),
            ]
        );

        // Check default profile patterns
        let default_fs = &uri_info.file_patterns[""];
        assert_eq!(
            default_fs,
            &vec![
                "/opt/anotherdir/anotherfile.properties".to_string(),
                "/opt/somewhere/custom.properties".to_string(),
                "/opt/somewhere/custom.yaml".to_string(),
                "/opt/somewhere/custom.yml".to_string(),
            ]
        );

        // Should have no classpath patterns
        assert!(
            uri_info.classpath_patterns.is_empty()
                || uri_info.classpath_patterns.values().all(|v| v.is_empty())
        );
    }
}
