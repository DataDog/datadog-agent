// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

pub mod fd;
pub mod maps;

use std::env;
use std::fs::{self, Metadata};
use std::path::{Path, PathBuf};
use std::sync::OnceLock;

static PROC_ROOT: OnceLock<PathBuf> = OnceLock::new();

pub fn root_path() -> &'static Path {
    PROC_ROOT.get_or_init(|| {
        if let Ok(v) = env::var("HOST_PROC") {
            return v.into();
        }

        if env::var("DOCKER_DD_AGENT").is_ok() && Path::new("/host").exists() {
            return "/host/proc".into();
        }

        "/proc".into()
    })
}

#[derive(Debug)]
pub struct Cmdline {
    cmdline: String,
    separator: char,
}

impl Cmdline {
    pub fn new(mut cmdline: String) -> Self {
        // Command lines from proc can have trailing null bytes if the process
        // has replaced part of it. Get the length of the string after trimming
        // trailing null bytes and truncate the original string so that we can
        // avoid an allocation to convert the result of trim_end_matches() to a
        // String.
        let trim_len = cmdline.trim_end_matches('\0').len();

        // This won't panic since trim_len should always lie on a char boundary.
        // There is no checked variant.
        cmdline.truncate(trim_len);

        // In cases where the process changes its command line (with gunicorn
        // -n, or puma), the replacement command line is packed into a single
        // string with spaces. We try to recognize this case and split the
        // string into separate arguments so that the individual parsers do not
        // need to special case this.
        //
        // Note that there could be edge cases where this doesn't work properly,
        // such as if the executable path itself contains a space _and_ the
        // replacement command line is packed into a single string with spaces.
        let mut args = cmdline.split_terminator('\0');
        let separator = if let (Some(first), None) = (args.next(), args.next())
            && first.contains(' ')
        {
            ' '
        } else {
            '\0'
        };

        Cmdline { cmdline, separator }
    }

    pub fn get(pid: i32) -> Result<Self, std::io::Error> {
        let path = root_path().join(pid.to_string()).join("cmdline");
        Ok(Self::new(fs::read_to_string(path)?))
    }

    pub fn args(&self) -> impl DoubleEndedIterator<Item = &str> {
        self.cmdline.split_terminator(self.separator)
    }

    pub fn is_empty(&self) -> bool {
        self.cmdline.is_empty()
    }
}

impl From<&str> for Cmdline {
    fn from(value: &str) -> Self {
        Self::new(value.to_string())
    }
}

impl From<&[&str]> for Cmdline {
    fn from(value: &[&str]) -> Self {
        let joined = value.join("\0");
        Self::new(joined)
    }
}

/// Creates a `Cmdline` instance from a list of command-line arguments.
///
/// This macro provides a convenient way to construct `Cmdline` objects for testing
/// and other scenarios where you need to simulate process command lines without
/// reading from `/proc/<pid>/cmdline`.
///
/// # Syntax
///
/// - `cmdline![]` - Creates an empty command line
/// - `cmdline!["arg1", "arg2", ...]` - Creates a command line with the given arguments
///
/// Arguments are joined with null bytes (`\0`) internally, matching the format
/// used by the Linux kernel in `/proc/<pid>/cmdline` files.
///
/// # Examples
///
/// ```skip
/// // Empty command line
/// let empty = cmdline![];
/// assert!(empty.is_empty());
///
/// // Java application command line
/// let java_app = cmdline!["java", "-jar", "app.jar"];
/// let args: Vec<&str> = java_app.args().collect();
/// assert_eq!(args, vec!["java", "-jar", "app.jar"]);
///
/// // Python script with arguments
/// let python_script = cmdline!["python3", "script.py", "--verbose"];
/// assert_eq!(python_script.args().next(), Some("python3"));
/// ```
#[macro_export]
macro_rules! cmdline {
    () => {
        Cmdline::new(String::new())
    };
    ($($arg:expr),* $(,)?) => { Cmdline::from(&[$($arg),*][..]) };
}

#[derive(Debug)]
pub struct Exe(pub PathBuf);

impl Exe {
    pub fn get(pid: i32) -> Result<Self, std::io::Error> {
        let path = root_path().join(pid.to_string()).join("exe");
        Ok(Exe(fs::read_link(path)?))
    }

    pub fn stat(pid: i32) -> Result<Metadata, std::io::Error> {
        let path = root_path().join(pid.to_string()).join("exe");
        fs::metadata(path)
    }

    pub fn basename(&self) -> Option<&str> {
        if let Some(name) = self.0.file_name() {
            name.to_str()
        } else {
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cmdline_normalization() {
        // Normal multi-arg cmdline (should not be normalized)
        let normal = Cmdline::new("python\0-u\0script.py".to_string());
        let args: Vec<&str> = normal.args().collect();
        assert_eq!(args, vec!["python", "-u", "script.py"]);

        // Packed args (should be normalized)
        let packed = Cmdline::new("python -u script.py".to_string());
        let args: Vec<&str> = packed.args().collect();
        assert_eq!(args, vec!["python", "-u", "script.py"]);

        // Single arg without spaces (should not be normalized)
        let single = Cmdline::new("python".to_string());
        let args: Vec<&str> = single.args().collect();
        assert_eq!(args, vec!["python"]);

        // Packed with trailing null bytes (gunicorn case)
        let trailing_nulls = Cmdline::new("gunicorn: master [foobar]\0\0\0\0".to_string());
        let args: Vec<&str> = trailing_nulls.args().collect();
        assert_eq!(args, vec!["gunicorn:", "master", "[foobar]"]);

        // Empty cmdline
        let empty = Cmdline::new("".to_string());
        assert_eq!(empty.args().count(), 0);

        // Leading null byte with spaces (edge case)
        let leading_null = Cmdline::new("\0foo bar".to_string());
        let args: Vec<&str> = leading_null.args().collect();
        assert_eq!(args, vec!["", "foo bar"]);

        // Trailing null bytes without packing
        let trailing_nulls = Cmdline::new("foo\0\0bar\0\0baz\0\0".to_string());
        let args: Vec<&str> = trailing_nulls.args().collect();
        assert_eq!(args, vec!["foo", "", "bar", "", "baz"]);
    }
}
