// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! This package implements Python service name generation.

use std::iter::{Peekable, successors};
use std::os::unix::ffi::OsStrExt;
use std::path::{Path, PathBuf};

use normalize_path::NormalizePath;

use crate::fs::SubDirFs;
use crate::procfs::Cmdline;
use crate::service_name::{
    DetectionContext, ServiceNameMetadata, ServiceNameSource, gunicorn, uvicorn,
};

pub fn extract_name(cmdline: &Cmdline, ctx: &mut DetectionContext) -> Option<ServiceNameMetadata> {
    let mut args = cmdline.args().peekable();

    // Skip the first arg (python executable)
    args.next()?;

    // When Gunicorn is invoked via its wrapper script the command line ends up
    // looking like the example below, so redirect to the Gunicorn detector for
    // this case:
    //  /usr/bin/python3 /usr/bin/gunicorn foo:app()
    //
    // Another case where we want to redirect to the Gunicorn detector is when
    // gunicorn replaces its command line with something like the below. Because
    // of the [ready], we end up here first instead of going directly to the
    // Gunicorn detector.
    //  [ready] gunicorn: worker [airflow-webserver]
    match Path::new(args.peek()?).file_name()?.as_bytes() {
        b"gunicorn" | b"gunicorn:" => {
            args.next(); // Skip the gunicorn executable
            return Some(gunicorn::extract_name_from_args(args, &ctx.envs));
        }
        b"uvicorn" => {
            args.next(); // Skip the uvicorn executable
            return uvicorn::extract_name_from_args(args);
        }
        _ => {}
    };

    match parse_args(args)? {
        ArgType::Module(module) => Some(ServiceNameMetadata::new(
            module,
            ServiceNameSource::CommandLine,
        )),
        ArgType::FileName(path) => {
            // Normalize paths to handle .. and . components (e.g., /../example.py -> /example.py)
            let path = path.normalize();
            let path = ctx.resolve_working_dir_relative_path(&path).unwrap_or(path);

            let metadata = ctx.fs.metadata(&path).ok()?;
            let is_file = !metadata.is_dir();

            if is_file {
                let parent = path.parent();
                if parent.is_none_or(|p| p.as_os_str().is_empty() || p.as_os_str() == "/") {
                    // Path is a root level file, return the file name
                    return Some(ServiceNameMetadata::new(
                        find_nearest_top_level(&path, ctx.fs),
                        ServiceNameSource::CommandLine,
                    ));
                }
            }

            if let Some(name) = deduce_package_name(&path, is_file, ctx.fs) {
                return Some(ServiceNameMetadata::new(name, ServiceNameSource::Python));
            }

            // If deduce_package_name didn't find anything, fall back to find_nearest_top_level
            // For directories, check the directory itself
            // For files, check the parent directory
            let check_path = if is_file { path.parent()? } else { &path };

            let name = find_nearest_top_level(check_path, ctx.fs);
            if name != "." && name != "/" && name != "bin" && name != "sbin" {
                return Some(ServiceNameMetadata::new(
                    name,
                    ServiceNameSource::CommandLine,
                ));
            }

            Some(ServiceNameMetadata::new(
                find_nearest_top_level(Path::new(path.file_name()?), ctx.fs),
                ServiceNameSource::CommandLine,
            ))
        }
    }
}

#[derive(Debug, PartialEq)]
enum ArgType {
    Module(String),
    FileName(PathBuf),
}

fn parse_args<'a>(mut args: Peekable<impl Iterator<Item = &'a str>>) -> Option<ArgType> {
    let args = args.by_ref();
    'args_loop: while let Some(arg) = args.next() {
        if let Some(arg) = arg.strip_prefix("--") {
            if arg == "check-hash-based-pycs" {
                // Only long arg with an argument.  CPython doesn't allow
                // including the argument with an equals sign in the same arg.
                args.next();
            }

            continue;
        }

        if let Some(arg) = arg.strip_prefix("-") {
            for (i, c) in arg.char_indices() {
                let rest = arg.get(i + c.len_utf8()..)?;
                match c {
                    // Everything after -c is a command and it terminates
                    // the option parsing.
                    'c' => return None,
                    'm' => {
                        return if rest.is_empty() {
                            Some(ArgType::Module(args.next()?.to_string()))
                        } else {
                            Some(ArgType::Module(rest.to_string()))
                        };
                    }
                    'X' | 'W' => {
                        // Takes an argument, either attached here or in the next arg.
                        if rest.is_empty() {
                            args.next();
                            continue 'args_loop;
                        } else {
                            break;
                        }
                    }
                    _ => continue,
                }
            }
        } else {
            return Some(ArgType::FileName(PathBuf::from(arg)));
        }
    }

    None
}

/// Walks up the directory tree collecting directory names while `__init__.py` exists.
/// Returns the package name (directories joined with '.') if a package was found.
/// If `is_file` is true and directories were traversed, appends the filename (without extension) to the package.
fn deduce_package_name(path: &Path, is_file: bool, fs: &SubDirFs) -> Option<String> {
    let mut traversed = Vec::new();

    // Start from the directory containing the file, or the directory itself
    let start_dir = if is_file { path.parent()? } else { path };

    let parents = successors(Some(start_dir), |path| {
        let parent = path.parent()?;
        if parent.as_os_str().is_empty() {
            None
        } else {
            Some(parent)
        }
    });

    for parent in parents {
        if !fs.exists(parent.join("__init__.py")) {
            break;
        }

        // Prepend the directory name to traversed list
        if let Some(dir_name) = parent.file_name().and_then(|n| n.to_str()) {
            traversed.insert(0, dir_name);
        }
    }

    // Return None if no package directories were found
    if traversed.is_empty() {
        return None;
    }

    // If path is a file, append the filename without extension
    if is_file && let Some(mod_name) = path.file_stem() {
        traversed.push(mod_name.to_str()?);
    }

    Some(traversed.join("."))
}

/// Finds the nearest top-level directory that contains Python files by walking up from the given path.
/// Returns the basename of that directory without the extension.
/// If the path is a file, returns the filename without the extension.
fn find_nearest_top_level(path: &Path, fs: &SubDirFs) -> String {
    // Walk up the directory tree, collecting directories that contain .py files.
    // Stop when we reach an empty path (from relative paths) or when there are no .py files.
    // If path is a file, take_while filters it out (has_py_files returns false for files),
    // so we fall back to the original path via unwrap_or.
    let last_with_py = successors(Some(path), |path| {
        let parent = path.parent()?;
        // Stop at empty paths (e.g., parent of "file.py" is "")
        if parent.as_os_str().is_empty() {
            None
        } else {
            Some(parent)
        }
    })
    .take_while(|current| has_py_files(current, fs))
    .last()
    .unwrap_or(path);

    // Get the file stem (basename without extension)
    last_with_py
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or("")
        .to_string()
}

/// Checks if a directory contains any .py files
/// Returns false if path is not a directory.
fn has_py_files(path: &Path, fs: &SubDirFs) -> bool {
    fs.read_dir(path)
        .ok()
        .map(|entries| {
            entries.filter_map(Result::ok).any(|entry| {
                entry
                    .file_name()
                    .to_str()
                    .is_some_and(|name| name.ends_with(".py"))
            })
        })
        .unwrap_or(false)
}

#[cfg(test)]
#[allow(clippy::expect_used, clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::test_utils::TestDataFs;
    use std::collections::HashMap;
    use std::path::PathBuf;

    fn test_ctx() -> (HashMap<String, String>, TestDataFs) {
        let fs = TestDataFs::new("python");
        let mut envs = HashMap::new();
        // Set PWD to root so that resolve_working_dir_relative_path can resolve absolute paths
        envs.insert("PWD".to_string(), "/".to_string());
        (envs, fs)
    }

    #[test]
    fn test_3rd_level_module_path() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "modules/m1/first/nice/package"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "m1.first.nice.package");
    }

    #[test]
    fn test_2nd_level_module_path() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "modules/m1/first/nice"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "m1.first.nice");
    }

    #[test]
    fn test_2nd_level_explicit_script_inside_module() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "modules/m1/first/nice/something.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "m1.first.nice.something");
    }

    #[test]
    fn test_1st_level_module_path() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "modules/m1/first"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "m1.first");
    }

    #[test]
    fn test_empty_module() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "modules/m2"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "m2");
    }

    #[test]
    fn test_main_in_a_dir() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "apps/app1"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app1");
    }

    #[test]
    fn test_script_in_inner_dir() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "apps/app2/cmd/run.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app2");
    }

    #[test]
    fn test_script_in_top_level_dir() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "apps/app2/setup.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "app2");
    }

    #[test]
    fn test_top_level_script() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "example.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "example");
    }

    #[test]
    fn test_root_level_script() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "/example.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "example");
    }

    #[test]
    fn test_root_level_script_with_dotdot() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "/../example.py"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "example");
    }

    #[test]
    fn test_script_in_bin() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python", "/usr/bin/pytest"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "pytest");
    }

    #[test]
    fn test_script_in_bin_with_u_flag() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        let cmdline = cmdline!["python3", "-u", "bin/WALinuxAgent.egg"];

        let result = extract_name(&cmdline, &mut ctx);
        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "WALinuxAgent");
    }

    #[test]
    fn test_parse_python_args() {
        let tests: Vec<(&str, Cmdline, Option<ArgType>)> = vec![
            ("empty args", cmdline![], None),
            ("dash only", cmdline!["-"], None),
            ("-B flag only", cmdline!["-B"], None),
            ("-X flag only", cmdline!["-X"], None),
            (
                "script.py",
                cmdline!["script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-u script.py",
                cmdline!["-u", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-B script.py",
                cmdline!["-B", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-E script.py",
                cmdline!["-E", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-OO script.py",
                cmdline!["-OO", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "script.py -OO",
                cmdline!["script.py", "-OO"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-u -B -E script.py",
                cmdline!["-u", "-B", "-E", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-W default app.py",
                cmdline!["-W", "default", "app.py"],
                Some(ArgType::FileName(PathBuf::from("app.py"))),
            ),
            (
                "-Wdefault app.py",
                cmdline!["-Wdefault", "app.py"],
                Some(ArgType::FileName(PathBuf::from("app.py"))),
            ),
            (
                "-c print('hello') app.py",
                cmdline!["-c", "print('hello')", "app.py"],
                None,
            ),
            (
                "-cprint('hello') app.py",
                cmdline!["-cprint('hello')", "app.py"],
                None,
            ),
            (
                "-m module app.py",
                cmdline!["-m", "module", "app.py"],
                Some(ArgType::Module("module".to_string())),
            ),
            (
                "-mmodule app.py",
                cmdline!["-mmodule", "app.py"],
                Some(ArgType::Module("module".to_string())),
            ),
            (
                "-BEmmodule -u script.py",
                cmdline!["-BEmmodule", "-u", "script.py"],
                Some(ArgType::Module("module".to_string())),
            ),
            (
                "--check-hash-based-pycs always app.py",
                cmdline!["--check-hash-based-pycs", "always", "app.py"],
                Some(ArgType::FileName(PathBuf::from("app.py"))),
            ),
            (
                "--foo script.py",
                cmdline!["--foo", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-BE script.py",
                cmdline!["-BE", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-BEs -u script.py",
                cmdline!["-BEs", "-u", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-BW ignore script.py",
                cmdline!["-BW", "ignore", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-X dev script.py",
                cmdline!["-X", "dev", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-Xdev script.py",
                cmdline!["-Xdev", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-W error::DeprecationWarning script.py",
                cmdline!["-W", "error::DeprecationWarning", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
            (
                "-Werror::DeprecationWarning script.py",
                cmdline!["-Werror::DeprecationWarning", "script.py"],
                Some(ArgType::FileName(PathBuf::from("script.py"))),
            ),
        ];

        for (name, cmdline, expected) in tests {
            let result = parse_args(cmdline.args().peekable());
            assert_eq!(result, expected, "Test case '{}' failed", name);
        }
    }
}
