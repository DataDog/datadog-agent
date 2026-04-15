// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use cap_std::fs::Dir;
#[cfg(feature = "java-archives")]
use flate2::read::DeflateDecoder;
#[cfg(feature = "java-archives")]
use rawzip::{CompressionMethod, FileReader, ZipArchive, ZipArchiveEntryWayfinder};
use std::io::{self, Read};
use std::path::{Path, PathBuf};
#[cfg(feature = "java-archives")]
use walkdir::WalkDir;

/// SubDirFs is like a standard filesystem, except that it allows
/// absolute paths to be passed in operations, and strips them to make
/// them relative to the root directory. It also prevents escaping the
/// root directory.
///
/// This is similar to Go's SubDirFS in the datadog-agent codebase, except that
/// it internally uses something (cap_std::fs::Dir) that is more similar to Go's
/// Root.FS than Dir.FS since it prevents escapes via symbolic links too.
pub struct SubDirFs {
    dir: Dir,
    #[cfg(feature = "java-archives")]
    root_path: PathBuf,
}

const MAX_PARSE_FILE_SIZE: u64 = 1024 * 1024; // 1 MiB

/// fixPath ensures that the specified path is stripped of the leading slash
/// (if any) so that it can be passed to cap_std functions. The cap_std
/// functions in turn ensure that the path does not escape the root directory.
fn fix_path<P: AsRef<Path>>(path: &P) -> &Path {
    let path = path.as_ref();
    path.strip_prefix("/").unwrap_or(path)
}

/// UnverifiedFile is a wrapper around cap_std::fs::File that prevents reading
/// the file contents until size verification has been performed via the verify() method.
/// This ensures compile-time enforcement of size verification for all file reads.
///
/// To read the file, call .verify(max_size) which returns an impl Read after
/// performing security checks (file type validation, size limits, TOCTOU protection).
pub struct UnverifiedFile(cap_std::fs::File);

impl UnverifiedFile {
    /// Verifies the file and returns a reader that can be used to read the contents.
    /// This performs security checks:
    /// - Ensures the file is a regular file (not a device file)
    /// - Validates the file size doesn't exceed max_size (default 1 MiB)
    /// - Provides TOCTOU protection by limiting the reader
    ///
    /// # Arguments
    /// * `max_size` - Optional maximum file size in bytes. If None, uses 1 MiB default.
    ///
    /// # Returns
    /// A reader that implements Read, limited to the verified size.
    pub fn verify(&self, max_size: Option<u64>) -> io::Result<impl Read + '_> {
        size_verified_reader(&self.0, max_size)
    }

    #[cfg(feature = "java-archives")]
    pub fn verify_zip(self) -> io::Result<UnverifiedZipArchive> {
        verified_zip_archive(self)
    }
}

#[cfg(feature = "java-archives")]
/// UnverifiedZipArchive is a wrapper around rawzip that prevents reading ZIP
/// entry contents until size verification has been performed.
pub struct UnverifiedZipArchive {
    archive: ZipArchive<FileReader>,
    buffer: Vec<u8>,
}

#[cfg(feature = "java-archives")]
impl UnverifiedZipArchive {
    fn new(archive: ZipArchive<FileReader>) -> Self {
        Self {
            archive,
            buffer: vec![0; rawzip::RECOMMENDED_BUFFER_SIZE],
        }
    }

    fn archive_entry_name(entry: &rawzip::ZipFileHeaderRecord<'_>) -> Option<String> {
        let file_path = entry.file_path();
        let raw_name = file_path.as_ref();
        std::str::from_utf8(raw_name).ok().map(ToString::to_string)
    }

    fn unverified_file(
        &self,
        wayfinder: ZipArchiveEntryWayfinder,
        size_hint: u64,
        compression_method: CompressionMethod,
    ) -> io::Result<UnverifiedZipFile<'_>> {
        let entry = self
            .archive
            .get_entry(wayfinder)
            .map_err(rawzip_error_to_io_error)?;
        Ok(UnverifiedZipFile {
            entry,
            size_hint,
            compression_method,
        })
    }

    /// Returns whether any entry name matches the predicate, stopping at the
    /// first match.
    pub fn any_entry_name<F>(&mut self, mut predicate: F) -> io::Result<bool>
    where
        F: FnMut(&str) -> bool,
    {
        let mut entries = self.archive.entries(&mut self.buffer);
        while let Some(entry) = entries.next_entry().map_err(rawzip_error_to_io_error)? {
            if let Some(name) = Self::archive_entry_name(&entry)
                && predicate(&name)
            {
                return Ok(true);
            }
        }

        Ok(false)
    }

    /// Reads the contents of the first exact-name entry found in archive order.
    pub fn read_file_to_vec(&mut self, name: &str, max_size: Option<u64>) -> io::Result<Vec<u8>> {
        let mut entries = self.archive.entries(&mut self.buffer);
        while let Some(entry) = entries.next_entry().map_err(rawzip_error_to_io_error)? {
            let Some((wayfinder, size_hint, compression_method)) = (|| {
                let entry_name = Self::archive_entry_name(&entry)?;
                if entry_name != name {
                    return None;
                }

                Some((
                    entry.wayfinder(),
                    entry.uncompressed_size_hint(),
                    entry.compression_method(),
                ))
            })() else {
                continue;
            };

            let file = self.unverified_file(wayfinder, size_hint, compression_method)?;
            let mut reader = file.verify(max_size)?;
            let mut contents = Vec::new();
            reader.read_to_end(&mut contents)?;
            return Ok(contents);
        }

        Err(io::Error::new(
            io::ErrorKind::NotFound,
            format!("missing ZIP entry: {name}"),
        ))
    }

    /// Scans entries in archive order and returns the first mapped value
    /// produced by a readable matching entry.
    pub fn find_map_file_contents<F, G, T>(
        &mut self,
        mut predicate: F,
        max_size: Option<u64>,
        mut map: G,
    ) -> io::Result<Option<T>>
    where
        F: FnMut(&str) -> bool,
        G: FnMut(&str, Vec<u8>) -> Option<T>,
    {
        let mut cd_buffer = vec![0; rawzip::RECOMMENDED_BUFFER_SIZE];
        let mut entries = self.archive.entries(&mut cd_buffer);
        while let Some(entry) = entries.next_entry().map_err(rawzip_error_to_io_error)? {
            let Some(name) = Self::archive_entry_name(&entry) else {
                continue;
            };
            if !predicate(&name) {
                continue;
            }

            let wayfinder = entry.wayfinder();
            let size_hint = entry.uncompressed_size_hint();
            let compression_method = entry.compression_method();
            let file = match self.unverified_file(wayfinder, size_hint, compression_method) {
                Ok(file) => file,
                Err(_) => continue,
            };
            let mut reader = match file.verify(max_size) {
                Ok(reader) => reader,
                Err(_) => continue,
            };
            let mut contents = Vec::new();
            if reader.read_to_end(&mut contents).is_err() {
                continue;
            }
            if let Some(value) = map(&name, contents) {
                return Ok(Some(value));
            }
        }

        Ok(None)
    }
}

#[cfg(feature = "java-archives")]
/// UnverifiedZipFile is a wrapper around a rawzip entry that prevents reading
/// the file contents until size verification has been performed via the verify() method.
/// This ensures compile-time enforcement of size verification for ZIP entry reads.
///
/// Metadata access (name, size) is allowed without verification.
pub struct UnverifiedZipFile<'a> {
    entry: rawzip::ZipEntry<'a, FileReader>,
    size_hint: u64,
    compression_method: CompressionMethod,
}

#[cfg(feature = "java-archives")]
impl<'a> UnverifiedZipFile<'a> {
    /// Verifies the ZIP entry and returns a reader that can be used to read the contents.
    pub fn verify(self, max_size: Option<u64>) -> io::Result<Box<dyn Read + 'a>> {
        let max_size = max_size.unwrap_or(MAX_PARSE_FILE_SIZE);
        size_verified_zip_reader(
            self.entry,
            self.size_hint,
            self.compression_method,
            max_size,
        )
    }
}

impl SubDirFs {
    /// Creates a new SubDirFs rooted at the specified path
    pub fn new<P: AsRef<Path>>(root: P) -> io::Result<Self> {
        let dir = Dir::open_ambient_dir(root.as_ref(), cap_std::ambient_authority())?;
        Ok(Self {
            dir,
            #[cfg(feature = "java-archives")]
            root_path: root.as_ref().to_path_buf(),
        })
    }

    /// Creates a new SubDirFs rooted at the specified path relative to the current root.
    #[cfg(feature = "java-archives")]
    pub fn sub<P: AsRef<Path>>(&self, path: P) -> io::Result<Self> {
        let fixed = fix_path(&path);
        let sub_path = self.root_path.join(fixed);
        let dir = Dir::open_ambient_dir(&sub_path, cap_std::ambient_authority())?;
        Ok(Self {
            dir,
            root_path: sub_path,
        })
    }

    /// Opens a file for reading, returning an UnverifiedFile.
    /// To read the file contents, call .verify() on the returned file.
    pub fn open<P: AsRef<Path>>(&self, path: P) -> io::Result<UnverifiedFile> {
        let fixed = fix_path(&path);
        let file = self.dir.open(fixed)?;
        Ok(UnverifiedFile(file))
    }

    /// Gets metadata for a file or directory
    pub fn metadata<P: AsRef<Path>>(&self, path: P) -> io::Result<cap_std::fs::Metadata> {
        let fixed = fix_path(&path);
        self.dir.metadata(fixed)
    }

    /// Reads a symbolic link
    ///
    /// We don't expose read_link because it returns an error if the link target
    /// is an absolute path.
    pub fn read_link_contents<P: AsRef<Path>>(&self, path: P) -> io::Result<PathBuf> {
        let fixed = fix_path(&path);
        self.dir.read_link_contents(fixed)
    }

    /// Reads a directory
    #[cfg_attr(not(test), allow(dead_code))]
    pub fn read_dir<P: AsRef<Path>>(&self, path: P) -> io::Result<cap_std::fs::ReadDir> {
        let fixed = fix_path(&path);
        self.dir.read_dir(fixed)
    }

    /// Gets symlink metadata (doesn't follow symlinks)
    pub fn symlink_metadata<P: AsRef<Path>>(&self, path: P) -> io::Result<cap_std::fs::Metadata> {
        let fixed = fix_path(&path);
        self.dir.symlink_metadata(fixed)
    }

    /// Returns `true` if the path points at an existing entity.
    /// This is a convenience method that is equivalent to `self.metadata(path).is_ok()`.
    pub fn exists<P: AsRef<Path>>(&self, path: P) -> bool {
        let fixed = fix_path(&path);
        self.dir.exists(fixed)
    }

    /// Returns a pre-configured WalkDir for the given start path.
    /// This allows callers to use walkdir's filter_entry and other features directly.
    ///
    /// Use `make_relative()` to convert the absolute paths from walkdir entries
    /// back to paths relative to SubDirFs root.
    #[cfg(feature = "java-archives")]
    pub fn walker(&self, start_path: &str) -> WalkDir {
        let full_path = self.root_path.join(fix_path(&start_path));
        WalkDir::new(full_path)
            .max_depth(16) // arbitrary depth limit to reduce computation
            .follow_links(false)
            .follow_root_links(false)
    }

    /// Converts an absolute path to a path relative to SubDirFs root.
    /// Returns None if the path is not within the SubDirFs root.
    ///
    /// This is useful when working with walkdir entries from `walker()`.
    #[cfg(feature = "java-archives")]
    pub fn make_relative(&self, path: &Path) -> Option<String> {
        let relative = path.strip_prefix(&self.root_path).ok()?;
        Some(if relative.as_os_str().is_empty() {
            ".".to_string()
        } else {
            relative.display().to_string()
        })
    }
}

#[cfg(feature = "java-archives")]
// We don't verify the size of the zip archive, the sizes of individual entries
// are verified when we read them via the type system enforcement.
fn verified_zip_archive(file: UnverifiedFile) -> io::Result<UnverifiedZipArchive> {
    let metadata = file.0.metadata()?;
    if !metadata.is_file() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "not a regular file",
        ));
    }

    let std_file = file.0.into_std();
    let mut buffer = vec![0; rawzip::RECOMMENDED_BUFFER_SIZE];
    let archive = ZipArchive::from_file(std_file, &mut buffer).map_err(rawzip_error_to_io_error)?;
    Ok(UnverifiedZipArchive::new(archive))
}

#[cfg(feature = "java-archives")]
/// Returns a reader for a ZIP entry after verifying the size doesn't exceed max_size.
fn size_verified_zip_reader(
    zip_file: rawzip::ZipEntry<'_, FileReader>,
    size_hint: u64,
    compression_method: CompressionMethod,
    max_size: u64,
) -> io::Result<Box<dyn Read + '_>> {
    if size_hint > max_size {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            format!(
                "ZIP file entry too large ({} bytes, max {} bytes)",
                size_hint, max_size
            ),
        ));
    }

    let compressed_reader = zip_file.reader();

    let verified_reader: Box<dyn Read + '_> = match compression_method {
        CompressionMethod::Store => Box::new(zip_file.verifying_reader(compressed_reader)),
        CompressionMethod::Deflate => {
            Box::new(zip_file.verifying_reader(DeflateDecoder::new(compressed_reader)))
        }
        unsupported => {
            return Err(io::Error::new(
                io::ErrorKind::InvalidData,
                format!("unsupported ZIP compression method: {:?}", unsupported),
            ));
        }
    };

    Ok(Box::new(verified_reader.take(size_hint.min(max_size))))
}

#[cfg(feature = "java-archives")]
fn rawzip_error_to_io_error(err: rawzip::Error) -> io::Error {
    io::Error::new(io::ErrorKind::InvalidData, err)
}

/// Returns a reader for the file after ensuring that the file is a regular file
/// and that the size that can be read from the reader will not exceed a
/// pre-defined safety limit to control memory usage.
///
/// This prevents reading device files, oversized files, and provides protection
/// against TOCTOU issues by using a LimitReader.
pub fn size_verified_reader(
    file: &cap_std::fs::File,
    max_size: Option<u64>,
) -> io::Result<impl Read + '_> {
    let metadata = file.metadata()?;

    let max_size = max_size.unwrap_or(MAX_PARSE_FILE_SIZE);

    // Don't try to read device files, etc.
    if !metadata.is_file() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "not a regular file",
        ));
    }

    let size = metadata.len();
    if size > max_size {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            format!("file too large ({} bytes)", size),
        ));
    }

    // Additional limit the reader to avoid surprises if the file size changes
    // while reading it (TOCTOU protection)
    Ok(file.take(size.min(max_size)))
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used, clippy::indexing_slicing)]
mod tests {
    use super::*;
    use crate::test_utils::TestDataFs;
    #[cfg(feature = "java-archives")]
    use std::io::Write;
    #[cfg(feature = "java-archives")]
    use tempfile::TempDir;

    #[test]
    fn test_fix_path() {
        assert_eq!(fix_path(&Path::new("/foo/bar")), Path::new("foo/bar"));
        assert_eq!(fix_path(&Path::new("foo/bar")), Path::new("foo/bar"));
    }

    #[test]
    fn test_subdirfs_operations() {
        // Use the nodejs test data directory
        let fs = TestDataFs::new("nodejs");
        let fs: &SubDirFs = fs.as_ref();

        // Test: can stat files with absolute paths (leading /)
        assert!(
            fs.metadata("/testdata/index.js").is_ok(),
            "Should be able to stat /testdata/index.js"
        );

        // Test: can stat files with relative paths
        assert!(
            fs.metadata("testdata/index.js").is_ok(),
            "Should be able to stat testdata/index.js"
        );

        // Test: prevents escaping with ../
        assert!(
            fs.metadata("../nodejs").is_err(),
            "Should not be able to escape with ../"
        );

        // Test: handles paths with .. in the middle (absolute)
        assert!(
            fs.metadata("/testdata/inner/../index.js").is_ok(),
            "Should be able to stat /testdata/inner/../index.js"
        );

        // Test: handles paths with .. in the middle (relative)
        assert!(
            fs.metadata("testdata/inner/../index.js").is_ok(),
            "Should be able to stat testdata/inner/../index.js"
        );

        // Test: can open files
        let file = fs.open("testdata/inner/../index.js");
        assert!(
            file.is_ok(),
            "Should be able to open testdata/inner/../index.js"
        );

        // Test: can read directories
        let entries = fs.read_dir("/testdata");
        assert!(
            entries.is_ok(),
            "Should be able to read directory /testdata"
        );

        let entries = entries.unwrap();
        let names: Vec<String> = entries
            .filter_map(|e| e.ok())
            .map(|e| e.file_name().to_string_lossy().to_string())
            .collect();

        assert!(
            names.contains(&"index.js".to_string()),
            "Directory should contain index.js, found: {:?}",
            names
        );
        assert!(
            names.contains(&"package.json".to_string()),
            "Directory should contain package.json, found: {:?}",
            names
        );

        // Test: exists method
        assert!(fs.exists("testdata/index.js"), "Should find existing file");
        assert!(
            fs.exists("/testdata/index.js"),
            "Should work with absolute paths"
        );
        assert!(
            !fs.exists("testdata/nonexistent.js"),
            "Should return false for missing file"
        );
    }

    #[test]
    fn test_size_verified_reader() {
        let testdata = TestDataFs::new("nodejs");
        let fs = SubDirFs::new(&testdata).expect("Failed to create SubDirFs");

        // Test: can read a normal file
        let file = fs.open("testdata/package.json").unwrap();
        let mut reader = file.verify(None).unwrap();
        let mut contents = String::new();
        reader.read_to_string(&mut contents).unwrap();
        assert!(contents.contains("my-awesome-package"));

        // Test: rejects files that are too large
        let file = fs.open("testdata/package.json").unwrap();
        let result = file.verify(Some(10)); // 10 bytes is too small
        assert!(result.is_err());
        if let Err(err) = result {
            assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
            assert!(err.to_string().contains("too large"));
        }

        // Test: limits reading to max_size even if file is larger
        let file = fs.open("testdata/package.json").unwrap();
        let file_size = file.0.metadata().unwrap().len();
        // Use a max_size that's larger than the check but smaller than actual file
        let max_size = file_size + 100; // Allow the file through size check
        let mut reader = file.verify(Some(max_size)).unwrap();
        let mut buffer = Vec::new();
        reader.read_to_end(&mut buffer).unwrap();
        // The reader should limit to the actual file size (not max_size)
        assert_eq!(buffer.len() as u64, file_size, "Should read entire file");
    }

    #[test]
    fn test_subdirfs_symlinks() {
        use std::fs;

        // Create a temporary directory for the test
        let tmpdir =
            std::env::temp_dir().join(format!("subdirfs_symlinks_test_{}", std::process::id()));
        fs::create_dir_all(&tmpdir).expect("Failed to create temp directory");

        // Ensure cleanup on test completion
        let _guard = scopeguard::guard(tmpdir.clone(), |path| {
            let _ = fs::remove_dir_all(path);
        });

        // Create test files and directories
        let subdir = tmpdir.join("subdir");
        fs::create_dir(&subdir).expect("Failed to create subdir");

        let target_file = tmpdir.join("target.txt");
        fs::write(&target_file, b"target content").expect("Failed to write target file");

        let nested_target = subdir.join("nested.txt");
        fs::write(&nested_target, b"nested content").expect("Failed to write nested file");

        // Create symlinks
        let link_to_file = tmpdir.join("link_to_file");
        std::os::unix::fs::symlink("target.txt", &link_to_file)
            .expect("Failed to create symlink to file");

        let link_to_nested = tmpdir.join("link_to_nested");
        std::os::unix::fs::symlink("subdir/nested.txt", &link_to_nested)
            .expect("Failed to create symlink to nested file");

        let link_with_dotdot = subdir.join("link_with_dotdot");
        std::os::unix::fs::symlink("../target.txt", &link_with_dotdot)
            .expect("Failed to create symlink with ..");

        let broken_link = tmpdir.join("broken_link");
        std::os::unix::fs::symlink("nonexistent.txt", &broken_link)
            .expect("Failed to create broken symlink");

        let absolute_link = tmpdir.join("absolute_link");
        std::os::unix::fs::symlink("/target.txt", &absolute_link)
            .expect("Failed to create absolute symlink");

        // Create SubDirFs
        let fs = SubDirFs::new(&tmpdir).expect("Failed to create SubDirFs");

        // Test: can read metadata of symlink target (follows symlink)
        let metadata = fs
            .metadata("link_to_file")
            .expect("Should be able to get metadata");
        assert!(metadata.is_file(), "Should follow symlink and find file");

        // Test: symlink_metadata doesn't follow symlinks
        let symlink_meta = fs
            .symlink_metadata("link_to_file")
            .expect("Should be able to get symlink metadata");
        assert!(symlink_meta.is_symlink(), "Should detect symlink");

        // Test: can read_link to get symlink target
        let target = fs
            .read_link_contents("link_to_file")
            .expect("Should be able to read symlink");
        assert_eq!(target, PathBuf::from("target.txt"));

        // Test: can read_link with absolute path
        let target = fs
            .read_link_contents("/link_to_file")
            .expect("Should work with absolute path");
        assert_eq!(target, PathBuf::from("target.txt"));

        // Test: can follow symlink to nested file
        let metadata = fs
            .metadata("link_to_nested")
            .expect("Should follow symlink to nested");
        assert!(
            metadata.is_file(),
            "Should find nested file through symlink"
        );

        let target = fs
            .read_link_contents("link_to_nested")
            .expect("Should read nested symlink");
        assert_eq!(target, PathBuf::from("subdir/nested.txt"));

        // Test: can follow symlink with .. in target
        let metadata = fs
            .metadata("subdir/link_with_dotdot")
            .expect("Should follow symlink with ..");
        assert!(metadata.is_file(), "Should resolve .. in symlink target");

        let target = fs
            .read_link_contents("subdir/link_with_dotdot")
            .expect("Should read symlink with ..");
        assert_eq!(target, PathBuf::from("../target.txt"));

        // Test: broken symlink - symlink_metadata should work but metadata should fail
        let symlink_meta = fs
            .symlink_metadata("broken_link")
            .expect("Should be able to stat broken symlink");
        assert!(symlink_meta.is_symlink(), "Should detect broken symlink");

        let target = fs
            .read_link_contents("broken_link")
            .expect("Should be able to read broken symlink");
        assert_eq!(target, PathBuf::from("nonexistent.txt"));

        assert!(
            fs.metadata("broken_link").is_err(),
            "Following broken symlink should fail"
        );

        // Test: symlink with absolute path target
        let target = fs
            .read_link_contents("absolute_link")
            .expect("Should be able to read absolute symlink");
        assert_eq!(target, PathBuf::from("/target.txt"));

        // Note that we can't follow the symlink via open() or metadata()
        // because the symlink points outside of the root directory and we can't
        // fixup that resolution since it's done inside the system call.
        //
        // let metadata = fs
        //     .metadata("absolute_link")
        //     .expect("Should be able to follow absolute symlink");
        // assert!(
        //     metadata.is_file(),
        //     "Should follow absolute symlink and find file"
        // );
        //
        // let mut file = fs
        //     .open("absolute_link")
        //     .expect("Should be able to open through absolute symlink");
        // let mut contents = String::new();
        // file.read_to_string(&mut contents)
        //     .expect("Should be able to read");
        // assert_eq!(
        //     contents, "target content",
        //     "Should read correct content through absolute symlink"
        // );
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_enforcement() {
        let mut archive = create_test_archive(create_test_zip_data(zip::CompressionMethod::Stored));

        assert!(archive.any_entry_name(|name| name == "small.txt").unwrap());

        let small = archive.read_file_to_vec("small.txt", Some(100)).unwrap();
        assert_eq!(small, b"small content");

        let large = archive.read_file_to_vec("large.txt", Some(100));
        assert!(large.is_err());
        if let Err(err) = large {
            assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
            assert!(err.to_string().contains("too large"));
        }
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_deflate() {
        let mut archive =
            create_test_archive(create_test_zip_data(zip::CompressionMethod::Deflated));

        let small = archive.read_file_to_vec("small.txt", Some(100)).unwrap();
        assert_eq!(small, b"small content");
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_any_entry_name_stops_on_first_match() {
        let mut archive = create_test_archive(create_zip_with_files(vec![
            ("match.txt", "matched"),
            ("later.txt", "later"),
        ]));

        assert!(archive.any_entry_name(|name| name == "match.txt").unwrap());
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_read_file_to_vec_returns_first_match() {
        let mut archive = create_test_archive(create_zip_with_files(vec![
            ("target.txt", "first"),
            ("other.txt", "other"),
        ]));

        let contents = archive.read_file_to_vec("target.txt", Some(100)).unwrap();
        assert_eq!(contents, b"first");
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_find_map_file_contents_returns_first_mapped_value() {
        let mut archive = create_test_archive(create_zip_with_files(vec![
            ("ignore.txt", "ignore"),
            ("config-1.txt", "first"),
            ("config-2.txt", "second"),
        ]));

        let value = archive
            .find_map_file_contents(
                |name| name.starts_with("config-"),
                Some(100),
                |name, contents| {
                    if name == "config-1.txt" {
                        return None;
                    }

                    Some(String::from_utf8(contents).unwrap())
                },
            )
            .unwrap();

        assert_eq!(value, Some("second".to_string()));
    }

    #[test]
    #[cfg(feature = "java-archives")]
    fn test_unverified_zip_archive_find_map_file_contents_skips_unreadable_match() {
        let mut archive = create_test_archive(create_zip_with_files(vec![
            ("config-too-large.txt", &"X".repeat(2000)),
            ("config-ok.txt", "usable"),
        ]));

        let value = archive
            .find_map_file_contents(
                |name| name.starts_with("config-"),
                Some(100),
                |_, contents| Some(String::from_utf8(contents).unwrap()),
            )
            .unwrap();

        assert_eq!(value, Some("usable".to_string()));
    }

    #[cfg(feature = "java-archives")]
    fn create_test_archive(data: Vec<u8>) -> UnverifiedZipArchive {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("test.zip");
        std::fs::write(&path, data).unwrap();
        let fs = SubDirFs::new(dir.path()).unwrap();
        fs.open("test.zip").unwrap().verify_zip().unwrap()
    }

    #[cfg(feature = "java-archives")]
    fn create_test_zip_data(compression_method: zip::CompressionMethod) -> Vec<u8> {
        let mut buf = Vec::new();
        {
            let mut writer = zip::ZipWriter::new(std::io::Cursor::new(&mut buf));
            let options: zip::write::FileOptions<()> =
                zip::write::FileOptions::default().compression_method(compression_method);

            writer.start_file("small.txt", options).unwrap();
            writer.write_all(b"small content").unwrap();

            writer.start_file("large.txt", options).unwrap();
            let large_content = "X".repeat(2000);
            writer.write_all(large_content.as_bytes()).unwrap();

            writer.finish().unwrap();
        }
        buf
    }

    #[cfg(feature = "java-archives")]
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
}
