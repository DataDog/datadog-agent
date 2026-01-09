// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

mod config;
mod locations;
mod pattern;
mod properties;
mod yaml;

use crate::fs::{SubDirFs, UnverifiedZipArchive};
use crate::procfs::Cmdline;
use crate::service_name::context::DetectionContext;
use config::extract_spring_boot_config;
use pattern::{longest_path_prefix, match_start, matches};
use properties::parse_properties;
use std::io::{self, BufRead, BufReader, Read};
use std::iter;
use std::path::{Path, PathBuf};
use yaml::parse_yaml;

const BOOT_INF_JAR_PATH: &str = "BOOT-INF/classes/";
const DEFAULT_LOCATIONS: &str = "optional:classpath:/;optional:classpath:/config/;optional:file:./;optional:file:./config/;optional:file:./config/*/";
const DEFAULT_CONFIG_NAME: &str = "application";
const APPNAME_PROP_NAME: &str = "spring.application.name";
const MANIFEST_FILE: &str = "META-INF/MANIFEST.MF";

/// Parse a property file, returning the value of the specified key
/// Returns Some(value) if found, None otherwise
fn parse_property_file<R: Read>(reader: R, filename: &str, target_key: &str) -> Option<String> {
    let ext = Path::new(filename)
        .extension()
        .and_then(|s| s.to_str())
        .map(|s| s.to_lowercase())
        .unwrap_or_default();

    match ext.as_str() {
        "properties" => parse_properties(reader, target_key),
        "yaml" | "yml" => parse_yaml(reader, target_key),
        _ => None,
    }
}

fn scan_property_from_filesystem(
    fs: &SubDirFs,
    patterns: &[String],
    property_key: &str,
) -> Option<String> {
    for pattern in patterns {
        let start_path = longest_path_prefix(pattern);

        // Use walkdir with filter_entry to prune directories that can't match
        for entry in fs
            .walker(start_path)
            .into_iter()
            .filter_entry(|e| {
                // Get path relative to SubDirFs root
                let Some(path) = fs.make_relative(e.path()) else {
                    return false;
                };

                match_start(pattern, &path)
            })
            .filter_map(Result::ok)
        {
            // Get path relative to SubDirFs root
            let Some(path) = fs.make_relative(entry.path()) else {
                continue;
            };

            // Only process files that match the full pattern
            if !entry.file_type().is_dir()
                && matches(pattern, &path)
                && let Some(value) = parse_property_file_from_fs(fs, &path, property_key)
            {
                return Some(value);
            }
        }
    }

    None
}

/// Parse a property file from the filesystem
fn parse_property_file_from_fs(
    fs: &SubDirFs,
    filename: &str,
    property_key: &str,
) -> Option<String> {
    let file = fs.open(filename).ok()?;
    let reader = file.verify(None).ok()?;

    parse_property_file(reader, filename, property_key)
}

/// Scan archive for a specific property, returning early when found
fn scan_property_from_archive<R: Read + io::Seek>(
    archive: &mut UnverifiedZipArchive<R>,
    patterns: &[String],
    property_key: &str,
) -> Option<String> {
    for i in 0..archive.len() {
        let Ok(file) = archive.by_index(i) else {
            continue;
        };

        let name = file.name().to_string();
        // Skip files not in BOOT-INF/classes
        if !name.starts_with(BOOT_INF_JAR_PATH) {
            continue;
        }

        if patterns.iter().any(|pattern| matches(pattern, &name))
            && let Ok(verified_reader) = file.verify(None)
            && let Some(value) = parse_property_file(verified_reader, &name, property_key)
        {
            return Some(value);
        }
    }

    None
}

/// Get Spring Boot application name with a callback for classpath sources
/// This matches the Go implementation's getSpringBootAppNameWithReader
fn get_spring_boot_app_name_with_reader<F>(
    ctx: &DetectionContext,
    args: impl Iterator<Item = impl AsRef<str>>,
    mut get_classpath_sources: F,
) -> Option<String>
where
    F: FnMut(&[String], &str) -> Option<String>,
{
    let config = extract_spring_boot_config(args, &ctx.envs);

    // Check if app name is already defined in args/system props/env
    if let Some(appname) = config.app_name {
        return Some(appname);
    }

    // Parse configuration locations
    let locations = config
        .config_location
        .as_deref()
        .unwrap_or(DEFAULT_LOCATIONS);
    let config_name = config.config_name.as_deref().unwrap_or(DEFAULT_CONFIG_NAME);

    let profiles: Vec<&str> = if let Some(raw_profile) = config.active_profiles.as_ref() {
        raw_profile.split(',').collect()
    } else {
        Vec::new()
    };

    let location_info = locations::parse(
        ctx,
        locations.split(';'),
        config_name,
        &profiles,
        BOOT_INF_JAR_PATH,
    );

    // Profiles are checked in order, with specific profiles taking precedence over default
    for profile in profiles.iter().chain(iter::once(&"")) {
        if let Some(patterns) = location_info.file_patterns.get(profile)
            && let Some(value) = scan_property_from_filesystem(ctx.fs, patterns, APPNAME_PROP_NAME)
        {
            return Some(value);
        }

        if let Some(patterns) = location_info.classpath_patterns.get(profile)
            && let Some(value) = get_classpath_sources(patterns, APPNAME_PROP_NAME)
        {
            return Some(value);
        }
    }

    None
}

/// Get Spring Boot application name from a JAR file
pub fn get_spring_boot_app_name(
    jarname: &str,
    ctx: &DetectionContext,
    cmdline: &Cmdline,
) -> Option<String> {
    let jarname = PathBuf::from(jarname);
    let abs_name = ctx
        .resolve_working_dir_relative_path(&jarname)
        .unwrap_or(jarname);

    log::debug!(
        "parsing information from spring boot archive: {:?}",
        abs_name
    );

    let file = ctx.fs.open(abs_name).ok()?;
    let mut archive = file.verify_zip().ok()?;

    get_spring_boot_app_name_from_jar(ctx, cmdline.args(), &mut archive)
}

fn get_spring_boot_app_name_from_jar<R: Read + io::Seek>(
    ctx: &DetectionContext,
    args: impl Iterator<Item = impl AsRef<str>>,
    archive: &mut UnverifiedZipArchive<R>,
) -> Option<String> {
    if !is_spring_boot_archive(archive) {
        return None;
    }

    get_spring_boot_app_name_with_reader(ctx, args, |patterns, property_key| {
        scan_property_from_archive(archive, patterns, property_key)
    })
}

/// Get Spring Boot application name from an unpacked JAR directory
fn get_spring_boot_app_name_from_unpacked_jar(
    ctx: &DetectionContext,
    args: impl Iterator<Item = impl AsRef<str>>,
    abs_path: &str,
) -> Option<String> {
    let abs_path = abs_path.to_string(); // Clone for use in closure
    // For unpacked JARs, we need to adjust classpath patterns by prepending the base path
    get_spring_boot_app_name_with_reader(ctx, args, move |patterns, property_key| {
        // Adjust patterns to include the base path for unpacked JAR
        let adjusted: Vec<String> = patterns
            .iter()
            .map(|p| format!("{}/{}", abs_path, p))
            .collect();

        // Scan filesystem with adjusted patterns
        scan_property_from_filesystem(ctx.fs, &adjusted, property_key)
    })
}

/// Get Spring Boot application name when launched via JarLauncher
pub fn get_spring_boot_launcher_app_name(
    ctx: &DetectionContext,
    cmdline: &Cmdline,
) -> Option<String> {
    // To limit the amount of processing, we only support the most common case
    // of having the files in the first entry in the classpath (or in the
    // default classpath if not explicit classpath is specified).
    let first_classpath = get_first_class_path(cmdline.args());

    let first_classpath = PathBuf::from(first_classpath);
    let base_path = ctx
        .resolve_working_dir_relative_path(&first_classpath)
        .unwrap_or(first_classpath);

    let is_jar = base_path.extension().and_then(|s| s.to_str()) == Some("jar");

    if is_jar {
        let file = ctx.fs.open(base_path).ok()?;
        let mut archive = file.verify_zip().ok()?;

        if let Some(name) = get_spring_boot_app_name_from_jar(ctx, cmdline.args(), &mut archive) {
            return Some(name);
        }

        // Fallback to manifest Start-Class
        if let Some(name) = get_start_class_name_from_jar(&mut archive) {
            return Some(name);
        }
    } else {
        let base_path_str = base_path.to_string_lossy().to_string();

        if let Some(name) =
            get_spring_boot_app_name_from_unpacked_jar(ctx, cmdline.args(), &base_path_str)
        {
            return Some(name);
        }

        // Fallback to manifest Start-Class
        let manifest_path = base_path.join(MANIFEST_FILE);
        if let Some(name) = get_start_class_name_from_file(ctx, manifest_path) {
            return Some(name);
        }
    }

    None
}

fn is_spring_boot_archive<R: Read + io::Seek>(archive: &mut UnverifiedZipArchive<R>) -> bool {
    for i in 0..archive.len() {
        if let Ok(file) = archive.by_index(i)
            && file.name().starts_with("BOOT-INF/")
        {
            return true;
        }
    }
    false
}

/// Get first class path from Java command line arguments
fn get_first_class_path<'a, I>(args: I) -> String
where
    I: IntoIterator<Item = &'a str>,
{
    // Skip the first argument (java command)
    let mut args = args.into_iter().skip(1);

    let mut cparg = None;

    while let Some(arg) = args.next() {
        // Check for classpath in various formats
        if arg == "-cp" || arg == "-classpath" || arg == "--class-path" {
            cparg = args.next();
            continue;
        }

        if let Some(stripped) = arg.strip_prefix("--class-path=") {
            cparg = Some(stripped);
            continue;
        }

        // Stop at -jar, -m, or --module
        if arg == "-jar" || arg == "-m" || arg == "--module" {
            break;
        }

        // Stop at classname (non-flag argument)
        if !arg.starts_with('-') {
            break;
        }

        // Skip flags that take parameters (e.g., --some-flag value)
        if arg.starts_with("--") && !arg.contains('=') {
            args.next(); // Consume the parameter
        }
    }

    let path = cparg
        .and_then(|paths| paths.split(':').next())
        .unwrap_or(".");
    if !path.is_empty() {
        return path.to_string();
    }

    ".".to_string()
}

/// Parse Start-Class from manifest file
fn parse_start_class<R: Read>(reader: R) -> Option<String> {
    let reader = BufReader::new(reader);
    for line in reader.lines() {
        if let Ok(line) = line
            && let Some(stripped) = line.strip_prefix("Start-Class: ")
        {
            return Some(stripped.trim().to_string());
        }
    }
    None
}

fn get_start_class_name_from_file(ctx: &DetectionContext, filename: PathBuf) -> Option<String> {
    let file = ctx.fs.open(filename).ok()?;
    let reader = file.verify(None).ok()?;

    parse_start_class(reader)
}

fn get_start_class_name_from_jar<R: Read + io::Seek>(
    archive: &mut UnverifiedZipArchive<R>,
) -> Option<String> {
    let file = archive.by_name(MANIFEST_FILE).ok()?;
    let reader = file.verify(None).ok()?;

    parse_start_class(reader)
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::fs::SubDirFs;
    use crate::procfs::Cmdline;
    use crate::test_utils::TestDataFs;
    use std::collections::HashMap;
    use std::io::Write;
    use tempfile::TempDir;
    use zip::ZipArchive;

    // Helper function to create a ZIP file with specific contents
    fn create_zip_with_files(files: Vec<(&str, &str)>) -> Vec<u8> {
        let mut buf = Vec::new();
        {
            let mut writer = zip::ZipWriter::new(std::io::Cursor::new(&mut buf));
            let options: zip::write::FileOptions<()> = zip::write::FileOptions::default()
                .compression_method(zip::CompressionMethod::Stored);

            for (name, content) in files {
                writer.start_file(name, options).unwrap();
                writer.write_all(content.as_bytes()).unwrap();
            }
            writer.finish().unwrap();
        }
        buf
    }

    #[test]
    fn test_is_spring_boot_archive_with_boot_inf_directory() {
        let buf = create_zip_with_files(vec![("MANIFEST.MF", ""), ("BOOT-INF/", "")]);

        let cursor = std::io::Cursor::new(buf);
        let archive = ZipArchive::new(cursor).unwrap();
        let mut archive = UnverifiedZipArchive::from_archive(archive);
        assert!(is_spring_boot_archive(&mut archive));
    }

    #[test]
    fn test_is_spring_boot_archive_with_boot_inf_file() {
        let buf = create_zip_with_files(vec![("BOOT-INF", "")]);

        let cursor = std::io::Cursor::new(buf);
        let archive = ZipArchive::new(cursor).unwrap();
        let mut archive = UnverifiedZipArchive::from_archive(archive);
        assert!(!is_spring_boot_archive(&mut archive));
    }

    #[test]
    fn test_is_spring_boot_archive_with_nested_boot_inf() {
        let buf = create_zip_with_files(vec![("MANIFEST.MF", ""), ("META-INF/BOOT-INF/", "")]);

        let cursor = std::io::Cursor::new(buf);
        let archive = ZipArchive::new(cursor).unwrap();
        let mut archive = UnverifiedZipArchive::from_archive(archive);
        assert!(!is_spring_boot_archive(&mut archive));
    }

    #[test]
    fn test_scan_property_from_archive() {
        let buf = create_zip_with_files(vec![
            (
                "BOOT-INF/classes/application.properties",
                "spring.application.name=default\nother.property=value1",
            ),
            (
                "BOOT-INF/classes/config/prod/application-prod.properties",
                "spring.application.name=prod\nother.property=value2",
            ),
        ]);

        let cursor = std::io::Cursor::new(buf);
        let archive = ZipArchive::new(cursor).unwrap();
        let mut archive = UnverifiedZipArchive::from_archive(archive);

        // Test finding default application.properties
        let patterns_default = vec![
            "BOOT-INF/classes/application.properties".to_string(),
            "BOOT-INF/classes/config/*/application.properties".to_string(),
        ];
        let result =
            scan_property_from_archive(&mut archive, &patterns_default, "spring.application.name");
        assert_eq!(result, Some("default".to_string()));

        // Test finding prod application.properties
        let cursor2 = std::io::Cursor::new(create_zip_with_files(vec![
            (
                "BOOT-INF/classes/application.properties",
                "spring.application.name=default",
            ),
            (
                "BOOT-INF/classes/config/prod/application-prod.properties",
                "spring.application.name=prod",
            ),
        ]));
        let archive2 = ZipArchive::new(cursor2).unwrap();
        let mut archive2 = UnverifiedZipArchive::from_archive(archive2);

        let patterns_prod = vec![
            "BOOT-INF/classes/application-prod.properties".to_string(),
            "BOOT-INF/classes/config/*/application-prod.properties".to_string(),
        ];
        let result2 =
            scan_property_from_archive(&mut archive2, &patterns_prod, "spring.application.name");
        assert_eq!(result2, Some("prod".to_string()));

        // Test property not found
        let result3 =
            scan_property_from_archive(&mut archive2, &patterns_prod, "non.existent.property");
        assert_eq!(result3, None);
    }

    #[test]
    fn test_scan_property_from_filesystem() {
        let fs = TestDataFs::new("spring");
        let fs: &SubDirFs = fs.as_ref();

        let patterns = vec![
            "application-fs.properties".to_string(),
            "testdata/*/application-fs.properties".to_string(),
        ];

        // Test finding property that exists
        let result = scan_property_from_filesystem(fs, &patterns, "spring.application.name");
        assert_eq!(result, Some("from-fs".to_string()));

        // Test finding property that doesn't exist
        let result2 = scan_property_from_filesystem(fs, &patterns, "non.existent.property");
        assert_eq!(result2, None);

        // Test with pattern that doesn't match any files
        let patterns_no_match = vec!["nonexistent.properties".to_string()];
        let result3 =
            scan_property_from_filesystem(fs, &patterns_no_match, "spring.application.name");
        assert_eq!(result3, None);
    }

    #[test]
    fn test_parse_property_file_extensions() {
        // Test YAML (not case sensitive)
        let yaml_content = "spring:\n  application:\n    name: test";
        let result = parse_property_file(
            std::io::Cursor::new(yaml_content),
            "test.YAmL",
            "spring.application.name",
        );
        assert_eq!(result, Some("test".to_string()));

        // Test properties file
        let props_content = "spring.application.name=test";
        let result2 = parse_property_file(
            std::io::Cursor::new(props_content),
            "test.properties",
            "spring.application.name",
        );
        assert_eq!(result2, Some("test".to_string()));

        // Test YML
        let yml_content = "spring:\n  application:\n    name: test";
        let result3 = parse_property_file(
            std::io::Cursor::new(yml_content),
            "TEST.YML",
            "spring.application.name",
        );
        assert_eq!(result3, Some("test".to_string()));

        // Test unknown extension
        let result4 = parse_property_file(
            std::io::Cursor::new(""),
            "unknown.extension",
            "spring.application.name",
        );
        assert_eq!(result4, None);
    }

    // Test removed: Expander functionality has been simplified out
    // to reduce memory allocations. Placeholder resolution is no longer supported.

    #[test]
    fn test_get_first_class_path() {
        let args = [
            "java",
            "-cp",
            "/path/to/app.jar:/path/to/lib.jar",
            "com.example.Main",
        ];
        let cp = get_first_class_path(args.iter().copied());
        assert_eq!(cp, "/path/to/app.jar");

        let args2 = ["java", "-jar", "app.jar"];
        let cp2 = get_first_class_path(args2.iter().copied());
        assert_eq!(cp2, ".");

        let args = ["java", "--class-path=", "app.jar"];
        let cp = get_first_class_path(args.iter().copied());
        assert_eq!(cp, ".");
    }

    // Properties and YAML parsing tests have been moved to their respective modules:
    // - properties.rs: test_parse_properties, test_parse_properties_streaming
    // - yaml.rs: test_parse_yaml, test_parse_yaml_streaming

    #[test]
    fn test_parse_start_class() {
        let manifest = "Manifest-Version: 1.0\nStart-Class: com.example.MyApp\n".as_bytes();
        let result = parse_start_class(std::io::Cursor::new(manifest));
        assert_eq!(result, Some("com.example.MyApp".to_string()));
    }

    // Helper to create a Spring Boot JAR for testing
    fn create_spring_boot_jar(tmp_dir: &TempDir, name: &str) -> String {
        let jar_path = tmp_dir.path().join(name);
        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);

        // Add Spring Boot structure
        writer.start_file("BOOT-INF/", options).unwrap();
        writer
            .start_file("BOOT-INF/classes/application.properties", options)
            .unwrap();
        writer
            .write_all(b"spring.application.name=default-app")
            .unwrap();
        writer
            .start_file(
                "BOOT-INF/classes/config/prod/application-prod.properties",
                options,
            )
            .unwrap();
        writer
            .write_all(b"spring.application.name=prod-app")
            .unwrap();
        writer
            .start_file(
                "BOOT-INF/classes/some/nested/location/application-yaml.yaml",
                options,
            )
            .unwrap();
        writer
            .write_all(b"spring:\n  application:\n    name: yaml-app")
            .unwrap();
        writer
            .start_file("BOOT-INF/classes/custom.properties", options)
            .unwrap();
        writer
            .write_all(b"spring.application.name=custom-app")
            .unwrap();

        writer.finish().unwrap();
        jar_path.to_string_lossy().to_string()
    }

    #[test]
    fn test_spring_boot_not_a_spring_boot_jar() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = tmp_dir.path().join("app.jar");
        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);
        writer.start_file("regular.file", options).unwrap();
        writer.finish().unwrap();

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-jar", "app.jar"];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path.to_string_lossy(), &ctx, &cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_spring_boot_with_app_name_as_arg() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-jar", &jar_path, "--spring.application.name=found"];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("found".to_string()));
    }

    #[test]
    fn test_spring_boot_with_app_name_as_system_property() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-Dspring.application.name=found", "-jar", &jar_path];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("found".to_string()));
    }

    #[test]
    fn test_spring_boot_with_app_name_as_env() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let mut envs = HashMap::new();
        envs.insert("SPRING_APPLICATION_NAME".to_string(), "found".to_string());
        let cmdline = cmdline!["java", "-jar", &jar_path];
        let ctx = DetectionContext::new(0, envs, &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("found".to_string()));
    }

    #[test]
    fn test_spring_boot_default_options() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-jar", &jar_path];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("default-app".to_string()));
    }

    #[test]
    fn test_spring_boot_prod_profile() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-Dspring.profiles.active=prod", "-jar", &jar_path];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        // prod profile file exists but default takes precedence in the location order
        assert_eq!(result, Some("default-app".to_string()));
    }

    #[test]
    fn test_spring_boot_custom_location() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline![
            "java",
            "-Dspring.config.locations=classpath:/**/location/",
            "-jar",
            &jar_path,
            "--spring.profiles.active=yaml"
        ];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("yaml-app".to_string()));
    }

    #[test]
    fn test_spring_boot_custom_config_name() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = create_spring_boot_jar(&tmp_dir, "app.jar");

        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline![
            "java",
            "-Dspring.config.name=custom",
            "-jar",
            &jar_path,
            "--spring.profiles.active=prod,yaml"
        ];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_app_name(&jar_path, &ctx, &cmdline);
        assert_eq!(result, Some("custom-app".to_string()));
    }

    #[test]
    fn test_spring_boot_launcher_with_props() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = tmp_dir.path().join("props.jar");
        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);

        writer.start_file("BOOT-INF/", options).unwrap();
        writer
            .start_file("BOOT-INF/classes/application.properties", options)
            .unwrap();
        writer.write_all(b"spring.application.name=my-app").unwrap();
        writer.finish().unwrap();

        let jar_path_str = jar_path.to_string_lossy().to_string();
        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-cp", &jar_path_str, "launcher"];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_launcher_app_name(&ctx, &cmdline);
        assert_eq!(result, Some("my-app".to_string()));
    }

    #[test]
    fn test_spring_boot_launcher_without_props() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = tmp_dir.path().join("noprops.jar");
        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);

        writer.start_file("BOOT-INF/", options).unwrap();
        writer.start_file("META-INF/MANIFEST.MF", options).unwrap();
        writer.write_all(b"Start-Class: org.my.class").unwrap();
        writer.finish().unwrap();

        let jar_path_str = jar_path.to_string_lossy().to_string();
        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-cp", &jar_path_str, "launcher"];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_launcher_app_name(&ctx, &cmdline);
        assert_eq!(result, Some("org.my.class".to_string()));
    }

    #[test]
    fn test_spring_boot_launcher_empty() {
        let tmp_dir = TempDir::new().unwrap();
        let jar_path = tmp_dir.path().join("empty.jar");
        let file = std::fs::File::create(&jar_path).unwrap();
        let mut writer = zip::ZipWriter::new(file);
        let options: zip::write::FileOptions<()> =
            zip::write::FileOptions::default().compression_method(zip::CompressionMethod::Stored);
        writer.start_file("BOOT-INF/", options).unwrap();
        writer.finish().unwrap();

        let jar_path_str = jar_path.to_string_lossy().to_string();
        let fs = SubDirFs::new("/").unwrap();
        let cmdline = cmdline!["java", "-cp", &jar_path_str, "launcher"];
        let ctx = DetectionContext::new(0, HashMap::new(), &fs);

        let result = get_spring_boot_launcher_app_name(&ctx, &cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_spring_boot_unpacked_jar_with_no_properties() {
        let fs = TestDataFs::new("spring");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "without-prop".to_string());
        let cmdline = cmdline!["java", "-jar", "launcher"];
        let ctx = DetectionContext::new(0, envs, fs.as_ref());

        let result = get_spring_boot_launcher_app_name(&ctx, &cmdline);
        assert_eq!(
            result,
            Some("com.example.spring_boot.ApplicationKtx".to_string())
        );
    }
}
