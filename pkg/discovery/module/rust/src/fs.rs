// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use cap_std::fs::Dir;
use flate2::read::DeflateDecoder;
use rawzip::{self, CompressionMethod};
use std::io::{self, Read};
use std::path::{Path, PathBuf};
#[cfg(feature = "java-archives")]
use walkdir::WalkDir;

pub use rawzip::ReaderAt;

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
    pub fn verify_zip(self) -> io::Result<UnverifiedZipArchive<rawzip::FileReader>> {
        verified_zip_archive(self)
    }
}

/// UnverifiedZipArchive uses rawzip to lazily scan the ZIP central directory
/// on demand, avoiding the large upfront allocation that the `zip` crate performs
/// when loading all entry metadata into memory.
///
/// To read an entry, call .by_name() to get an UnverifiedZipFile,
/// then call .verify(max_size) on it.
pub struct UnverifiedZipArchive<R: ReaderAt>(rawzip::ZipArchive<R>);

impl<R: ReaderAt> UnverifiedZipArchive<R> {
    /// Gets a ZIP entry by name, returning an UnverifiedZipFile.
    /// This scans the central directory looking for a matching entry.
    /// To read the entry contents, call .verify(max_size) on the returned file.
    pub fn by_name(&self, name: &str) -> io::Result<UnverifiedZipFile<'_, R>> {
        self.find_entry(|n| n == name)?.ok_or_else(|| {
            io::Error::new(
                io::ErrorKind::NotFound,
                format!("entry '{}' not found in ZIP archive", name),
            )
        })
    }

    /// Finds the first entry matching the predicate by scanning the central directory.
    pub fn find_entry(
        &self,
        predicate: impl Fn(&str) -> bool,
    ) -> io::Result<Option<UnverifiedZipFile<'_, R>>> {
        let mut buf = vec![0u8; rawzip::RECOMMENDED_BUFFER_SIZE];
        let mut entries = self.0.entries(&mut buf);
        while let Some(header) = entries.next_entry().map_err(rawzip_to_io)? {
            let name = zip_entry_name(&header);
            if predicate(&name) {
                return Ok(Some(UnverifiedZipFile {
                    archive: &self.0,
                    wayfinder: header.wayfinder(),
                    compression_method: header.compression_method(),
                    name,
                    uncompressed_size: header.uncompressed_size_hint(),
                }));
            }
        }
        Ok(None)
    }

    /// Finds all entries matching the predicate by scanning the central directory.
    /// Returns lightweight handles that can be used to read entry contents later.
    pub fn find_entries(
        &self,
        predicate: impl Fn(&str) -> bool,
    ) -> io::Result<Vec<UnverifiedZipFile<'_, R>>> {
        let mut buf = vec![0u8; rawzip::RECOMMENDED_BUFFER_SIZE];
        let mut entries = self.0.entries(&mut buf);
        let mut result = Vec::new();
        while let Some(header) = entries.next_entry().map_err(rawzip_to_io)? {
            let name = zip_entry_name(&header);
            if predicate(&name) {
                result.push(UnverifiedZipFile {
                    archive: &self.0,
                    wayfinder: header.wayfinder(),
                    compression_method: header.compression_method(),
                    name,
                    uncompressed_size: header.uncompressed_size_hint(),
                });
            }
        }
        Ok(result)
    }

    /// Returns the number of files in the archive (from the end-of-central-directory record).
    #[allow(dead_code)]
    pub fn len(&self) -> usize {
        self.0.entries_hint() as usize
    }
}

#[cfg(test)]
impl UnverifiedZipArchive<std::io::Cursor<Vec<u8>>> {
    pub fn from_data(data: Vec<u8>) -> io::Result<Self> {
        let len = data.len() as u64;
        let cursor = std::io::Cursor::new(data);
        let mut buf = vec![0u8; rawzip::RECOMMENDED_BUFFER_SIZE];
        let archive = rawzip::ZipLocator::new()
            .locate_in_reader(cursor, &mut buf, len)
            .map_err(|(_, e)| rawzip_to_io(e))?;
        Ok(UnverifiedZipArchive(archive))
    }
}

/// UnverifiedZipFile holds a deferred reference to a ZIP entry. The entry's
/// compressed data is not read until .verify() is called.
///
/// Metadata access (name, size) is allowed without verification.
pub struct UnverifiedZipFile<'a, R: ReaderAt> {
    archive: &'a rawzip::ZipArchive<R>,
    wayfinder: rawzip::ZipArchiveEntryWayfinder,
    compression_method: CompressionMethod,
    name: String,
    uncompressed_size: u64,
}

impl<'a, R: ReaderAt> UnverifiedZipFile<'a, R> {
    /// Verifies the ZIP entry size and returns a reader for the decompressed contents.
    pub fn verify(self, max_size: Option<u64>) -> io::Result<impl Read + 'a> {
        let max_size = max_size.unwrap_or(MAX_PARSE_FILE_SIZE);

        if self.uncompressed_size > max_size {
            return Err(io::Error::new(
                io::ErrorKind::InvalidInput,
                format!(
                    "ZIP file entry too large ({} bytes, max {} bytes)",
                    self.uncompressed_size, max_size
                ),
            ));
        }

        let entry = self
            .archive
            .get_entry(self.wayfinder)
            .map_err(rawzip_to_io)?;
        let reader = entry.reader();

        match self.compression_method {
            CompressionMethod::Store => Ok(ZipEntryReader::Store(reader.take(max_size))),
            CompressionMethod::Deflate => Ok(ZipEntryReader::Deflate(
                DeflateDecoder::new(reader).take(max_size),
            )),
            other => Err(io::Error::new(
                io::ErrorKind::InvalidData,
                format!("unsupported ZIP compression method: {:?}", other),
            )),
        }
    }

    /// Returns the name of the file in the archive.
    pub fn name(&self) -> &str {
        &self.name
    }
}

/// Reader enum that dispatches between stored (uncompressed) and deflated ZIP entries.
enum ZipEntryReader<R: Read> {
    Store(io::Take<R>),
    Deflate(io::Take<DeflateDecoder<R>>),
}

impl<R: Read> Read for ZipEntryReader<R> {
    fn read(&mut self, buf: &mut [u8]) -> io::Result<usize> {
        match self {
            Self::Store(r) => r.read(buf),
            Self::Deflate(r) => r.read(buf),
        }
    }
}

fn rawzip_to_io(e: rawzip::Error) -> io::Error {
    io::Error::other(e.to_string())
}

fn zip_entry_name(header: &rawzip::ZipFileHeaderRecord<'_>) -> String {
    let path = header.file_path();
    String::from_utf8_lossy(path.as_bytes()).into_owned()
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
fn verified_zip_archive(
    file: UnverifiedFile,
) -> io::Result<UnverifiedZipArchive<rawzip::FileReader>> {
    let metadata = file.0.metadata()?;
    if !metadata.is_file() {
        return Err(io::Error::new(
            io::ErrorKind::InvalidInput,
            "not a regular file",
        ));
    }

    let std_file = file.0.into_std();
    let mut buf = vec![0u8; rawzip::RECOMMENDED_BUFFER_SIZE];
    let archive = rawzip::ZipArchive::from_file(std_file, &mut buf).map_err(rawzip_to_io)?;
    Ok(UnverifiedZipArchive(archive))
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
#[allow(clippy::unwrap_used, clippy::expect_used)]
pub(crate) mod tests {
    use super::*;
    use crate::test_utils::TestDataFs;
    #[cfg(feature = "java-archives")]
    use std::io::Write;

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
        // Test 1: Can access metadata without verification
        let archive = UnverifiedZipArchive::from_data(create_test_zip_data()).unwrap();
        let file = archive.by_name("small.txt").unwrap();
        assert_eq!(file.name(), "small.txt");
        assert_eq!(file.uncompressed_size, 13); // "small content" is 13 bytes

        // Test 2: Can verify and read small file
        let archive = UnverifiedZipArchive::from_data(create_test_zip_data()).unwrap();
        let file = archive.by_name("small.txt").unwrap();
        let mut reader = file.verify(Some(100)).unwrap();
        let mut contents = String::new();
        reader.read_to_string(&mut contents).unwrap();
        assert_eq!(contents, "small content");

        // Test 3: Verification rejects files that exceed max_size
        let archive = UnverifiedZipArchive::from_data(create_test_zip_data()).unwrap();
        let file = archive.by_name("large.txt").unwrap();
        let result = file.verify(Some(100)); // 100 bytes max, but file is 2000 bytes
        assert!(result.is_err());
        if let Err(err) = result {
            assert_eq!(err.kind(), std::io::ErrorKind::InvalidInput);
            assert!(err.to_string().contains("too large"));
        }

        // Test 4: find_entry works
        let archive = UnverifiedZipArchive::from_data(create_test_zip_data()).unwrap();
        let file = archive.find_entry(|n| n == "small.txt").unwrap().unwrap();
        assert_eq!(file.name(), "small.txt");
        let mut reader = file.verify(Some(100)).unwrap();
        let mut contents = String::new();
        reader.read_to_string(&mut contents).unwrap();
        assert_eq!(contents, "small content");
    }

    // Helper function for test_unverified_zip_archive_enforcement
    #[cfg(feature = "java-archives")]
    pub(crate) fn create_test_zip_data() -> Vec<u8> {
        create_zip_with_files(&[
            ("small.txt", b"small content"),
            ("large.txt", &[b'X'; 2000]),
        ])
    }

    #[cfg(feature = "java-archives")]
    pub(crate) fn create_zip_with_files(files: &[(&str, &[u8])]) -> Vec<u8> {
        let mut buf = Vec::new();
        {
            let mut writer = zip::ZipWriter::new(std::io::Cursor::new(&mut buf));
            let options: zip::write::FileOptions<()> = zip::write::FileOptions::default()
                .compression_method(zip::CompressionMethod::Stored);

            for (name, content) in files {
                writer.start_file(*name, options).unwrap();
                writer.write_all(content).unwrap();
            }
            writer.finish().unwrap();
        }
        buf
    }
}
