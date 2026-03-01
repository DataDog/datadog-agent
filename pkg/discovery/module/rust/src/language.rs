// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

extern crate lru;

use std::ffi::OsStr;
use std::fs::File;
use std::io::{Read, Seek, SeekFrom};
use std::num::NonZeroUsize;
use std::os::unix::fs::MetadataExt;
use std::path::Path;
use std::sync::{LazyLock, Mutex};

use anyhow::Result;
use elf::abi::{PF_W, PF_X, PT_LOAD};
use elf::endian::AnyEndian;
use log::info;
use lru::LruCache;
use memchr::memmem;
use serde::{Deserialize, Serialize};

use crate::procfs::{Cmdline, Exe, fd::OpenFilesInfo};

const BINARY_CACHE_SIZE: usize = 1000;

#[derive(Copy, Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Language {
    #[default]
    Unknown,
    #[serde(rename = "jvm")]
    Java,
    NodeJS,
    Python,
    Ruby,
    DotNet,
    Go,
    #[serde(rename = "cpp")]
    CPlusPlus,
    PHP,
}

impl Language {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Unknown => "unknown",
            Self::Java => "jvm",
            Self::NodeJS => "nodejs",
            Self::Python => "python",
            Self::Ruby => "ruby",
            Self::DotNet => "dotnet",
            Self::Go => "go",
            Self::CPlusPlus => "cpp",
            Self::PHP => "php",
        }
    }

    pub fn detect(pid: i32, exe: &Exe, cmdline: &Cmdline, open_files_info: &OpenFilesInfo) -> Self {
        info!("detect: exe={exe:?} cmdline={cmdline:?}");
        if let Some(lang) = Self::from_basename(cmdline) {
            return lang;
        }

        if let Some(lang) = Self::from_cmdline(cmdline) {
            return lang;
        }

        if let Some(lang) = Self::from_exe(exe) {
            return lang;
        }

        if let Some(lang) = Self::from_binary(pid, open_files_info) {
            return lang;
        }

        Self::Unknown
    }

    fn from_basename(cmdline: &Cmdline) -> Option<Self> {
        let mut args = cmdline.args();
        let exe = Path::new(args.next()?).file_name()?;
        info!("from_basename: exe from args: {exe:?}");

        if is_jruby(exe, cmdline) {
            return Some(Self::Ruby);
        }

        None
    }

    fn from_cmdline(cmdline: &Cmdline) -> Option<Self> {
        Self::from_command(get_exe(cmdline)?)
    }

    fn from_exe(exe: &Exe) -> Option<Self> {
        Self::from_command(exe.basename()?)
    }

    fn from_command(comm: &str) -> Option<Self> {
        info!("from_command: {comm}");
        match comm {
            "py" | "python" => return Some(Self::Python),
            "java" => return Some(Self::Java),
            "npm" | "node" => return Some(Self::NodeJS),
            "dotnet" => return Some(Self::DotNet),
            "ruby" | "rubyw" => return Some(Self::Ruby),
            "php" | "php-fpm" => return Some(Self::PHP),
            _ => {}
        }

        if comm.starts_with("python") {
            info!("from_command: {comm} -> python");
            return Some(Self::Python);
        }
        if comm.starts_with("java") && comm != "javac" {
            info!("from_command: {comm} -> java");
            return Some(Self::Java);
        }

        if is_ruby_prefix(comm) {
            info!("from_command: {comm} -> ruby");
            return Some(Self::Ruby);
        }
        if is_php_prefix(comm) {
            info!("from_command: {comm} -> php");
            return Some(Self::PHP);
        }

        None
    }

    /// Try language detection methods that are tied to a specific binary and
    /// can be cached at a binary level.
    fn from_binary(pid: i32, open_files_info: &OpenFilesInfo) -> Option<Self> {
        #[allow(clippy::unwrap_used)] // `BINARY_CACHE_SIZE` is a non-zero constant, this cannot fail
        static CACHE: LazyLock<Mutex<LruCache<BinaryID, Language>>> = LazyLock::new(|| {
            Mutex::new(LruCache::new(NonZeroUsize::new(BINARY_CACHE_SIZE).unwrap()))
        });

        let bin_id = BinaryID::get(pid).ok()?;

        CACHE
            .lock()
            .ok()?
            .try_get_or_insert(bin_id, || {
                if let Some(lang) = Self::from_injector(open_files_info) {
                    Ok(lang)
                } else if let Some(lang) = Self::from_go(pid) {
                    Ok(lang)
                } else if let Some(lang) = Self::from_dotnet(pid) {
                    Ok(lang)
                } else {
                    Err(())
                }
            })
            .ok()
            .cloned()
    }

    fn from_injector(open_files_info: &OpenFilesInfo) -> Option<Self> {
        use std::fs::File;
        use std::io::Read;

        const MEMFD_MAX_SIZE: usize = 10;

        let memfd_path = open_files_info.memfd_path.as_ref()?;

        let metadata = std::fs::metadata(memfd_path).ok()?;
        if !metadata.is_file() {
            return None;
        }

        let mut file = File::open(memfd_path).ok()?;
        let mut lang = [0u8; MEMFD_MAX_SIZE];
        let n = file.read(&mut lang).ok()?;

        match lang.get(..n)? {
            b"nodejs" | b"js" | b"node" => Some(Self::NodeJS),
            b"php" => Some(Self::PHP),
            b"jvm" | b"java" => Some(Self::Java),
            b"python" => Some(Self::Python),
            b"ruby" => Some(Self::Ruby),
            b"dotnet" => Some(Self::DotNet),
            _ => None,
        }
    }

    /// Detects Go binaries by trying to find either the `.go.buildinfo` section
    /// in ELF, or find the go buildinfo magic at the beginning of the data
    /// segment.
    ///
    /// This logic is ported from the datadog-agent
    /// (`pkg/network/go/binversion`), itself imported from Go's
    /// `debug/buildinfo` standard library package.
    fn from_go(pid: i32) -> Option<Self> {
        const ELF_READ_LIMIT: usize = 64 * 1024; // 64KiB

        const BUILD_INFO_MAGIC: &[u8] = b"\xff Go buildinf:";
        const BUILD_INFO_SIZE: usize = 32;
        const BUILD_INFO_ALIGN: usize = 16;

        let exe = Exe::get(pid).ok()?;
        let mut elf_file = File::open(&exe.0).ok()?;
        let mut elf = elf::ElfStream::<AnyEndian, _>::open_stream(&mut elf_file).ok()?;

        // First, try to find .go.buildinfo section.
        if elf.section_header_by_name(".go.buildinfo").ok()?.is_some() {
            return Some(Self::Go);
        }

        // Sometimes, there is no buildinfo section, and Go's buildinfo are somewhere
        // near the beginning of the data segment.
        let data_phdr = elf
            .segments()
            .iter()
            .find(|e| e.p_type == PT_LOAD && e.p_flags & (PF_X | PF_W) == PF_W)?;

        // Read up to ELF_READ_LIMIT bytes from the data segment
        let mut segment_buffer = [0u8; ELF_READ_LIMIT];
        let read_size = std::cmp::min(data_phdr.p_filesz as usize, ELF_READ_LIMIT);

        // Reopen file for manual read (elf consumes the original file)
        let mut file = File::open(&exe.0).ok()?;
        file.seek(SeekFrom::Start(data_phdr.p_offset)).ok()?;
        file.read_exact(segment_buffer.get_mut(..read_size)?).ok()?;

        let mut data = segment_buffer.get(..read_size)?;
        let finder = memmem::Finder::new(BUILD_INFO_MAGIC);
        loop {
            let i = finder.find(data)?;

            if data.len() - i < BUILD_INFO_SIZE {
                return None;
            }

            if i % BUILD_INFO_ALIGN == 0 && data.len() - i >= BUILD_INFO_SIZE {
                break;
            }
            data = data.get((i + BUILD_INFO_ALIGN - 1) & !(BUILD_INFO_ALIGN - 1)..)?
        }

        Some(Self::Go)
    }

    /// Detects .NET processes by scanning /proc/PID/maps for System.Runtime.dll.
    /// This works for non-single-file deployments (both self-contained and
    /// framework-dependent), and framework-dependent single-file deployments.
    /// It does not work for self-contained single-file deployments since these
    /// do not have any DLLs in their maps file.
    fn from_dotnet(pid: i32) -> Option<Self> {
        let maps_reader = crate::procfs::maps::get_reader_for_pid(pid).ok()?;

        if has_dotnet_dll_in_maps(maps_reader) {
            Some(Language::DotNet)
        } else {
            None
        }
    }
}

fn has_dotnet_dll_in_maps<R: std::io::BufRead>(maps_reader: R) -> bool {
    const DOTNET_RUNTIME_DLL: &str = "/System.Runtime.dll";

    maps_reader.lines().any(|line| {
        let Ok(line) = line else {
            return false;
        };

        line.ends_with(DOTNET_RUNTIME_DLL)
    })
}

#[derive(Eq, PartialEq, Hash)]
struct BinaryID {
    dev: u64,
    ino: u64,
}

impl BinaryID {
    fn get(pid: i32) -> Result<Self> {
        let stat = Exe::stat(pid)?;

        Ok(Self {
            dev: stat.dev(),
            ino: stat.ino(),
        })
    }
}

/// Matches ruby version patterns like "ruby3.1", "ruby2.7", "ruby10.15"
/// Equivalent to regex: ^ruby\d+\.\d+$
fn is_ruby_prefix(comm: &str) -> bool {
    // Minimum length of "ruby<digit>.<digit>" is 7
    if comm.len() < 7 || !comm.starts_with("ruby") {
        return false;
    }

    let Some(version_part) = comm.get(4..) else {
        return false;
    };

    // Find the dot separator
    let Some(dot_pos) = version_part.find('.') else {
        return false;
    };

    let (Some(major), Some(minor)) = (
        version_part.get(..dot_pos).filter(|m| !m.is_empty()),
        version_part.get(dot_pos + 1..).filter(|m| !m.is_empty()),
    ) else {
        return false;
    };

    // Check that major & minor versions are all digits
    major.chars().all(|c| c.is_ascii_digit()) && minor.chars().all(|c| c.is_ascii_digit())
}

/// Matches php version patterns like "php8", "php8.1", "php-fpm8", "php-fpm8.1"
/// Equivalent to regex: ^php(?:-fpm)?\d(?:\.\d)?$
fn is_php_prefix(comm: &str) -> bool {
    let remainder = if let Some(rest) = comm.strip_prefix("php") {
        rest
    } else {
        return false;
    };

    // Handle optional "-fpm" part
    let remainder = if let Some(rest) = remainder.strip_prefix("-fpm") {
        rest
    } else {
        remainder
    };

    if remainder.is_empty() {
        return false;
    }

    // Must start with a digit
    let mut chars = remainder.chars();
    match chars.next() {
        Some(c) if c.is_ascii_digit() => {}
        _ => return false,
    }

    // Check if there's more content
    let rest: String = chars.collect();
    if rest.is_empty() {
        // Just "php8" or "php-fpm8" format
        return true;
    }

    // Must be ".digit" format
    if !rest.starts_with('.') {
        return false;
    }

    rest.get(1..).is_some_and(|minor| {
        minor.len() == 1 && minor.chars().next().is_some_and(|c| c.is_ascii_digit())
    })
}

fn is_jruby(exe: &OsStr, cmdline: &Cmdline) -> bool {
    if !exe.eq_ignore_ascii_case("java") {
        return false;
    }

    // Check if any argument is "org.jruby.Main"
    cmdline.args().any(|arg| arg.trim() == "org.jruby.Main")
}

fn get_exe(cmdline: &Cmdline) -> Option<&str> {
    let mut exe = cmdline.args().next()?;

    // trim any quotes from the executable
    exe = exe.trim_matches('"');

    // Extract executable from commandline args
    exe = Path::new(exe).file_name()?.to_str()?;
    exe = exe.trim_matches(|c: char| !c.is_alphanumeric());

    if exe.is_empty() { None } else { Some(exe) }
}

#[cfg(test)]
#[allow(
    clippy::expect_used,
    clippy::print_stderr,
    clippy::undocumented_unsafe_blocks
)]
mod tests {
    use std::ffi::OsStr;

    use crate::cmdline;

    use super::*;

    macro_rules! assert_lang {
        ($lang:expr, $result:expr) => {
            assert_eq!(Some($lang), $result)
        };
    }

    macro_rules! assert_none {
        ($result:expr) => {
            assert_eq!(None, $result)
        };
    }

    #[test]
    fn test_is_jruby() {
        // Test case: java with org.jruby.Main should return true
        let jruby_cmdline = cmdline!["java", "-cp", "/path", "org.jruby.Main", "script.rb"];
        assert!(is_jruby(OsStr::new("java"), &jruby_cmdline));

        // Test case: case insensitive java executable matching
        assert!(is_jruby(OsStr::new("JAVA"), &jruby_cmdline));
        assert!(is_jruby(OsStr::new("Java"), &jruby_cmdline));

        // Test case: org.jruby.Main with whitespace should work
        let jruby_with_whitespace = cmdline!["java", "org.jruby.Main", "script.rb"];
        assert!(is_jruby(OsStr::new("java"), &jruby_with_whitespace));

        // Test case: java without org.jruby.Main should return false
        let regular_java = cmdline!["java", "-jar", "app.jar"];
        assert!(!is_jruby(OsStr::new("java"), &regular_java));

        // Test case: non-java executables should return false
        assert!(!is_jruby(OsStr::new("ruby"), &jruby_cmdline));
        assert!(!is_jruby(OsStr::new(""), &jruby_cmdline));
    }

    #[test]
    fn test_get_exe() {
        // Test case: normal multi-arg command line
        let normal_cmdline = cmdline!["java", "-jar", "app.jar"];
        assert_eq!(get_exe(&normal_cmdline), Some("java"));

        // Test case: single argument with spaces (all packed into first arg)
        let packed_cmdline = cmdline!["java", "-jar", "app.jar"];
        assert_eq!(get_exe(&packed_cmdline), Some("java"));

        // Test case: executable with quotes
        let quoted_cmdline = cmdline!["\"/usr/bin/java\"", "-jar", "app.jar"];
        assert_eq!(get_exe(&quoted_cmdline), Some("java"));

        // Test case: full path executable
        let path_cmdline = cmdline!["/usr/bin/java", "-jar", "app.jar"];
        assert_eq!(get_exe(&path_cmdline), Some("java"));

        // Test case: executable with non-alphanumeric characters (hyphen preserved)
        let special_cmdline = cmdline!["java-8", "-jar", "app.jar"];
        assert_eq!(get_exe(&special_cmdline), Some("java-8"));

        // Test case: complex path with special characters
        let complex_cmdline = cmdline!["/opt/java-11/bin/java", "-jar", "app.jar"];
        assert_eq!(get_exe(&complex_cmdline), Some("java"));

        // Test case: empty cmdline
        let empty_cmdline = cmdline![""];
        assert_eq!(get_exe(&empty_cmdline), None);

        // Test case: only non-alphanumeric characters
        let symbols_cmdline = cmdline!["---", "arg"];
        assert_eq!(get_exe(&symbols_cmdline), None);
    }

    #[test]
    fn test_from_command() {
        // Exact matches
        assert_lang!(Language::Python, Language::from_command("py"));
        assert_lang!(Language::Python, Language::from_command("python"));
        assert_lang!(Language::Java, Language::from_command("java"));
        assert_lang!(Language::NodeJS, Language::from_command("npm"));
        assert_lang!(Language::NodeJS, Language::from_command("node"));
        assert_lang!(Language::DotNet, Language::from_command("dotnet"));
        assert_lang!(Language::Ruby, Language::from_command("ruby"));
        assert_lang!(Language::Ruby, Language::from_command("rubyw"));
        assert_lang!(Language::PHP, Language::from_command("php"));
        assert_lang!(Language::PHP, Language::from_command("php-fpm"));

        // Python prefix matching
        assert_lang!(Language::Python, Language::from_command("python3"));
        assert_lang!(Language::Python, Language::from_command("python3.9"));
        assert_lang!(Language::Python, Language::from_command("python3.11"));
        assert_lang!(Language::Python, Language::from_command("python2.7"));

        // Java prefix matching with javac exclusion
        assert_lang!(Language::Java, Language::from_command("java8"));
        assert_lang!(Language::Java, Language::from_command("java11"));
        assert_lang!(Language::Java, Language::from_command("java17"));
        assert_lang!(Language::Java, Language::from_command("java21"));
        assert_none!(Language::from_command("javac"));

        // Ruby regex patterns
        assert_lang!(Language::Ruby, Language::from_command("ruby3.1"));
        assert_lang!(Language::Ruby, Language::from_command("ruby2.7"));
        assert_lang!(Language::Ruby, Language::from_command("ruby10.15"));
        // Ruby patterns
        assert_none!(Language::from_command("ruby3"));
        assert_none!(Language::from_command("ruby3.1.2"));

        // PHP regex patterns
        assert_lang!(Language::PHP, Language::from_command("php8"));
        assert_lang!(Language::PHP, Language::from_command("php8.1"));
        assert_lang!(Language::PHP, Language::from_command("php-fpm8"));
        assert_lang!(Language::PHP, Language::from_command("php-fpm8.1"));
        assert_lang!(Language::PHP, Language::from_command("php7"));
        assert_lang!(Language::PHP, Language::from_command("php7.4"));
        assert_none!(Language::from_command("php8.1.2"));
        assert_none!(Language::from_command("phpunit"));

        // Unknown languages
        assert_none!(Language::from_command("gcc"));
        assert_none!(Language::from_command("unknown"));
        assert_none!(Language::from_command(""));
    }

    #[test]
    fn test_has_dotnet_dll_in_maps() {
        use std::io::Cursor;

        use super::has_dotnet_dll_in_maps;

        // Test case: empty maps
        let maps = "";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!has_dotnet_dll_in_maps(reader));

        // Test case: maps without System.Runtime.dll
        let maps = "79f6cd47d000-79f6cd47f000 r--p 00000000 fc:04 793163                     /usr/lib/python3.10/lib-dynload/_bz2.cpython-310-x86_64-linux-gnu.so
79f6cd479000-79f6cd47a000 r-xp 00001000 fc:06 5507018                    /home/foo/.local/lib/python3.10/site-packages/ddtrace_fake/md.cpython-310-x86_64-linux-gnu.so";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!has_dotnet_dll_in_maps(reader));

        // Test case: maps with System.Runtime.dll
        let maps = "7d97b4e57000-7d97b4e85000 r--s 00000000 fc:04 1332568                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/System.Console.dll
7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/System.Runtime.dll
7d97b4e8e000-7d97b4e99000 r--p 00000000 fc:04 1332718                    /usr/lib/dotnet/shared/Microsoft.NETCore.App/8.0.8/libSystem.Native.so";
        let reader = Cursor::new(maps.as_bytes());
        assert!(has_dotnet_dll_in_maps(reader));

        // Test case: partial match should not detect (must end with /System.Runtime.dll)
        let maps = "7d97b4e85000-7d97b4e8e000 r--s 00000000 fc:04 1332665                    /usr/lib/dotnet/System.Runtime.dll.bak";
        let reader = Cursor::new(maps.as_bytes());
        assert!(!has_dotnet_dll_in_maps(reader));
    }

    #[test]
    fn test_from_dotnet() {
        use std::fs::File;

        use memmap2::Mmap;

        let current_pid = std::process::id().cast_signed();

        // Negative test: current process should NOT be detected as .NET initially
        let result = Language::from_dotnet(current_pid);
        assert_eq!(
            result, None,
            "Process should not be detected as .NET before mmapping System.Runtime.dll"
        );

        // Create path to test DLL
        let dll_path = crate::test_utils::testdata_path().join("System.Runtime.dll");

        // Memory-map the System.Runtime.dll file
        let file = File::open(&dll_path).expect("Failed to open System.Runtime.dll test file");
        let _mmap = unsafe { Mmap::map(&file).expect("Failed to mmap System.Runtime.dll") };

        // Positive test: current process SHOULD be detected as .NET after mmapping
        let result = Language::from_dotnet(current_pid);
        assert_eq!(
            result,
            Some(Language::DotNet),
            "Process should be detected as .NET after mmapping System.Runtime.dll"
        );

        // Keep mmap alive until the end of the test
        drop(_mmap);
    }

    #[test]
    #[cfg(target_arch = "x86_64")]
    fn test_from_go() {
        use std::process::{Command, Stdio};

        let go_binary = crate::test_utils::testdata_path().join("go_dummy_prog/test_binary");

        // Spawn a Go process with stdin/stdout piped so we can synchronize
        let child = Command::new(&go_binary)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .spawn()
            .expect("Failed to spawn Go test process");

        // Guard to ensure process cleanup even if test panics
        let mut child = scopeguard::guard(child, |mut c| {
            c.kill().ok();
            c.wait().ok();
        });

        let pid = child.id().cast_signed();

        // Wait for the "READY" signal from the Go process to ensure it's fully started
        let stdout = child.stdout.as_mut().expect("Failed to get stdout");
        let mut reader = std::io::BufReader::new(stdout);
        let mut ready_line = String::new();
        use std::io::BufRead;
        reader
            .read_line(&mut ready_line)
            .expect("Failed to read ready signal");
        assert_eq!(ready_line.trim(), "READY", "Process did not signal ready");

        // Test: Go binary should be detected as Go
        let result = Language::from_go(pid);
        assert_eq!(
            result,
            Some(Language::Go),
            "Go test binary should be detected as Go language"
        );
    }

    #[test]
    fn test_from_go_with_non_go_binary() {
        // Test with current process (Rust binary) - should NOT be detected as Go
        let current_pid = std::process::id().cast_signed();
        let result = Language::from_go(current_pid);
        assert_eq!(
            result, None,
            "Rust test binary should not be detected as Go"
        );
    }

    #[test]
    fn test_from_injector_with_valid_languages() {
        use std::io::Write;
        use tempfile::NamedTempFile;

        use crate::procfs::fd::OpenFilesInfo;

        let test_cases = vec![
            ("nodejs", Language::NodeJS),
            ("js", Language::NodeJS),
            ("node", Language::NodeJS),
            ("php", Language::PHP),
            ("jvm", Language::Java),
            ("java", Language::Java),
            ("python", Language::Python),
            ("ruby", Language::Ruby),
            ("dotnet", Language::DotNet),
        ];

        for (input, expected) in test_cases {
            let mut tmpfile = NamedTempFile::new().expect("Failed to create temp file");
            tmpfile
                .write_all(input.as_bytes())
                .expect("Failed to write to temp file");
            tmpfile.flush().expect("Failed to flush temp file");

            let open_files_info = OpenFilesInfo {
                sockets: vec![],
                logs: vec![],
                tracer_memfd: None,
                memfd_path: Some(tmpfile.path().to_path_buf()),
                has_gpu_device: false,
            };

            let result = Language::from_injector(&open_files_info);
            assert_eq!(
                result,
                Some(expected),
                "Expected {} to be detected as {:?}",
                input,
                expected
            );
        }
    }

    #[test]
    fn test_from_injector_with_unknown_language() {
        use std::io::Write;
        use tempfile::NamedTempFile;

        use crate::procfs::fd::OpenFilesInfo;

        let mut tmpfile = NamedTempFile::new().expect("Failed to create temp file");
        tmpfile
            .write_all(b"unknown_lang")
            .expect("Failed to write to temp file");
        tmpfile.flush().expect("Failed to flush temp file");

        let open_files_info = OpenFilesInfo {
            sockets: vec![],
            logs: vec![],
            tracer_memfd: None,
            memfd_path: Some(tmpfile.path().to_path_buf()),
            has_gpu_device: false,
        };

        let result = Language::from_injector(&open_files_info);
        assert_eq!(result, None, "Unknown language should return None");
    }

    #[test]
    fn test_from_injector_with_no_memfd() {
        use crate::procfs::fd::OpenFilesInfo;

        let open_files_info = OpenFilesInfo {
            sockets: vec![],
            logs: vec![],
            tracer_memfd: None,
            memfd_path: None,
            has_gpu_device: false,
        };

        let result = Language::from_injector(&open_files_info);
        assert_eq!(result, None, "No memfd_path should return None");
    }

    #[test]
    fn test_from_injector_with_non_regular_file() {
        use crate::procfs::fd::OpenFilesInfo;
        use std::path::PathBuf;

        // Try to use /dev/null which is not a regular file
        let open_files_info = OpenFilesInfo {
            sockets: vec![],
            logs: vec![],
            tracer_memfd: None,
            memfd_path: Some(PathBuf::from("/dev/null")),
            has_gpu_device: false,
        };

        let result = Language::from_injector(&open_files_info);
        assert_eq!(
            result, None,
            "Non-regular file should return None due to is_file() check"
        );
    }

    #[test]
    fn test_from_injector_with_nonexistent_file() {
        use crate::procfs::fd::OpenFilesInfo;
        use std::path::PathBuf;

        let open_files_info = OpenFilesInfo {
            sockets: vec![],
            logs: vec![],
            tracer_memfd: None,
            memfd_path: Some(PathBuf::from("/nonexistent/file/path")),
            has_gpu_device: false,
        };

        let result = Language::from_injector(&open_files_info);
        assert_eq!(result, None, "Nonexistent file should return None");
    }
}
