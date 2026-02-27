// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use crate::service_name::{DetectionContext, ServiceNameMetadata, ServiceNameSource};
use std::io::Read;
use std::path::{Path, PathBuf};

/// Checks if a file path has a JavaScript extension
fn is_js(path: &Path) -> bool {
    if let Some(ext) = path.extension()
        && let Some(ext_str) = ext.to_str()
    {
        let ext_lower = ext_str.to_lowercase();
        return ext_lower == "js" || ext_lower == "mjs" || ext_lower == "cjs";
    }
    false
}

/// Extracts the Node.js service name from the command line
pub fn extract_name(
    cmdline: &crate::procfs::Cmdline,
    ctx: &mut DetectionContext,
) -> Option<ServiceNameMetadata> {
    let mut args = cmdline.args();

    // Skip the first arg (node executable)
    args.next()?;

    let mut skip_next = false;
    for arg in args {
        if skip_next {
            skip_next = false;
            continue;
        }

        // Skip flags
        if arg.starts_with('-') {
            // Handle -r/--require flags specially
            if arg == "-r" || arg == "--require" {
                // Next arg is the required module, skip it
                // But only if the value isn't in this arg (e.g., -r=module)
                skip_next = !arg.contains('=');
                continue;
            }
            // Skip other flags
            continue;
        }

        // This is a potential entry point
        let path = PathBuf::from(arg);
        let path = ctx.resolve_working_dir_relative_path(&path).unwrap_or(path);

        let entry_point = if is_js(&path) {
            path
        } else if let Ok(metadata) = ctx.fs.symlink_metadata(&path)
            && metadata.is_symlink()
            && let Ok(target) = ctx.fs.read_link_contents(&path)
        {
            if !is_js(&target) {
                continue;
            }

            make_absolute(target, path.parent())
        } else {
            continue;
        };

        if ctx.fs.metadata(&entry_point).is_err() {
            continue;
        }

        // Try to find package.json starting from the search path
        if let Some(name) = find_package_json_name(&entry_point, ctx) {
            return Some(ServiceNameMetadata::new(name, ServiceNameSource::Nodejs));
        }

        // Fall back to the script/link name
        // Use file_stem() to get filename without extension
        return Some(ServiceNameMetadata::new(
            Path::new(arg).file_stem()?.to_string_lossy(),
            ServiceNameSource::CommandLine,
        ));
    }

    None
}

fn make_absolute(path: PathBuf, base: Option<&Path>) -> PathBuf {
    if path.is_absolute() {
        return path;
    }

    match base {
        Some(base) => base.join(path),
        None => path,
    }
}

/// Finds and extracts the name from the nearest package.json
fn find_package_json_name(entry_point: &Path, ctx: &DetectionContext) -> Option<String> {
    let mut current = entry_point.parent()?.to_path_buf();

    loop {
        let package_json_path = current.join("package.json");

        // Try to open the file
        if let Ok(file) = ctx.fs.open(&package_json_path) {
            // File exists, try to parse it
            if let Some(name) = parse_package_json(&file, &package_json_path)
                && !name.is_empty()
            {
                return Some(name);
            }
            // Found package.json but couldn't parse or no name, stop searching
            return None;
        }
        // File doesn't exist, continue searching up the directory tree

        // Move up one directory
        let parent = current.parent()?;
        if parent == current {
            // Reached the root
            break;
        }
        current = parent.to_path_buf();
    }

    None
}

/// Parses package.json and extracts the "name" field
/// Returns Some(name) if successfully parsed with a name field, None otherwise
fn parse_package_json(file: &crate::fs::UnverifiedFile, path: &Path) -> Option<String> {
    // Get a size-verified reader
    let mut reader = match file.verify(None) {
        Ok(r) => r,
        Err(e) => {
            log::debug!("Skipping package.json at {}: {}", path.display(), e);
            return None;
        }
    };

    // Read the file contents
    let mut contents = String::new();
    if reader.read_to_string(&mut contents).is_err() {
        log::debug!("Unable to read package.json at {}", path.display());
        return None;
    }

    // Parse JSON and extract the "name" field
    match serde_json::from_str::<serde_json::Value>(&contents) {
        Ok(json) => json
            .get("name")
            .and_then(|v| v.as_str())
            .map(|s| s.to_string()),
        Err(e) => {
            log::debug!("Unable to parse package.json at {}: {}", path.display(), e);
            None
        }
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::procfs::Cmdline;
    use crate::test_utils::TestDataFs;
    use std::collections::HashMap;

    fn test_ctx() -> (HashMap<String, String>, TestDataFs) {
        let testdata = crate::test_utils::TestDataFs::new("nodejs");
        let envs = HashMap::new();
        (envs, testdata)
    }

    #[test]
    fn test_is_js() {
        assert!(is_js(Path::new("index.js")));
        assert!(is_js(Path::new("server.mjs")));
        assert!(is_js(Path::new("app.cjs")));
        assert!(is_js(Path::new("/path/to/file.js")));
        assert!(is_js(Path::new("FILE.JS"))); // case insensitive

        assert!(!is_js(Path::new("index.ts")));
        assert!(!is_js(Path::new("package.json")));
        assert!(!is_js(Path::new("node")));
        assert!(!is_js(Path::new("")));
    }

    #[test]
    fn test_nodejs_with_valid_package_json() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["/usr/bin/node", "./testdata/index.js"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-awesome-package");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }

    #[test]
    fn test_nodejs_with_require_flag() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline![
            "/usr/bin/node",
            "--require",
            "/private/node-patches_legacy/register.js",
            "--preserve-symlinks-main",
            "--",
            "./testdata/index.js"
        ];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-awesome-package");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }

    #[test]
    fn test_nodejs_cjs_extension() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["/usr/bin/node", "./testdata/foo.cjs"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-awesome-package");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }

    #[test]
    fn test_nodejs_mjs_extension() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["/usr/bin/node", "./testdata/bar.mjs"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        assert_eq!(metadata.name, "my-awesome-package");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }

    #[test]
    fn test_nodejs_broken_package_json() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["/usr/bin/node", "./testdata/inner/app.js"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        // Should fall back to filename since package.json has no name field
        assert_eq!(metadata.name, "app");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_nodejs_no_package_json() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline![
            "/usr/bin/node",
            "--require",
            "/private/node-patches_legacy/register.js",
            "--preserve-symlinks-main",
            "--",
            "/somewhere/index.js"
        ];

        let result = extract_name(&cmdline, &mut ctx);
        // Should return None since /somewhere/index.js doesn't exist
        assert!(result.is_none());
    }

    #[test]
    fn test_nodejs_symlink() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["/usr/bin/node", "./testdata/inner/link"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        // link -> app.js, and inner/ has no package.json name, so falls back to filename
        assert_eq!(metadata.name, "link");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_nodejs_symlink_in_bins() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline![
            "/usr/bin/node",
            "--foo",
            "./testdata/bins/notjs",
            "--bar",
            "./testdata/bins/broken",
            "./testdata/bins/json-server"
        ];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());

        let metadata = result.unwrap();
        // json-server is a symlink to ../json-server/lib/bin.js
        assert_eq!(metadata.name, "json-server-package");
        assert_eq!(metadata.source, ServiceNameSource::Nodejs);
    }
}
