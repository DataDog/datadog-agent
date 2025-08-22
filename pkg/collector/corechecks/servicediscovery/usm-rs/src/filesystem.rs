use crate::error::{UsmError, UsmResult};
use std::collections::HashMap;
use std::io::{self, Read, Seek};
use std::path::{Path, PathBuf};
use walkdir::WalkDir;

/// A trait that combines Read + Seek for file operations
pub trait ReadSeek: Read + Seek + Send {}

impl<T: Read + Seek + Send> ReadSeek for T {}

/// File metadata information
#[derive(Debug, Clone)]
pub struct FileInfo {
    pub size: u64,
    pub is_dir: bool,
    pub is_file: bool,
}

/// Abstract file system interface
pub trait FileSystem: Send + Sync + std::fmt::Debug {
    /// Open a file for reading
    fn open(&self, path: &Path) -> UsmResult<Box<dyn Read + Send>>;
    
    /// Open a file for reading with seeking capability (for ZIP archives)
    fn open_seekable(&self, path: &Path) -> UsmResult<Box<dyn ReadSeek>>;
    
    /// Get file metadata
    fn metadata(&self, path: &Path) -> UsmResult<FileInfo>;
    
    /// Check if a file exists
    fn exists(&self, path: &Path) -> bool;
    
    /// List directory contents
    fn read_dir(&self, path: &Path) -> UsmResult<Vec<PathBuf>>;
    
    /// Read symlink target
    fn read_link(&self, path: &Path) -> UsmResult<PathBuf>;
    
    /// Find files matching a glob pattern
    fn glob(&self, base: &Path, pattern: &str) -> UsmResult<Vec<PathBuf>>;
    
    /// Create a sub-filesystem rooted at the given path
    fn sub(&self, root: &Path) -> UsmResult<Box<dyn FileSystem>>;
    
    /// Canonicalize path - resolve all symlinks and relative components
    fn canonicalize(&self, path: &Path) -> UsmResult<PathBuf>;
    
    /// Resolve working directory relative path
    fn resolve_working_dir_relative(&self, path: &str, cwd: &Path) -> PathBuf;
    
    /// Follow symlink chain to final target with loop detection
    fn resolve_symlink_chain(&self, path: &Path) -> UsmResult<PathBuf>;
}

/// Real filesystem implementation
#[derive(Debug, Default)]
pub struct RealFileSystem {
    root: PathBuf,
}

impl RealFileSystem {
    pub fn new() -> Self {
        Self {
            root: PathBuf::from("/"),
        }
    }
    
    pub fn with_root<P: AsRef<Path>>(root: P) -> Self {
        Self {
            root: root.as_ref().to_path_buf(),
        }
    }
    
    fn resolve_path(&self, path: &Path) -> PathBuf {
        if path.is_absolute() {
            // For absolute paths, strip the leading slash and join with root
            let relative = path.strip_prefix("/").unwrap_or(path);
            self.root.join(relative)
        } else {
            self.root.join(path)
        }
    }
    
    /// Manual path canonicalization when std::fs::canonicalize fails
    fn manual_canonicalize(&self, path: &PathBuf) -> UsmResult<PathBuf> {
        let mut result = PathBuf::new();
        
        for component in path.components() {
            match component {
                std::path::Component::RootDir => {
                    result.push(std::path::Component::RootDir);
                }
                std::path::Component::CurDir => {
                    // Skip current directory references
                }
                std::path::Component::ParentDir => {
                    result.pop();
                }
                std::path::Component::Normal(_) => {
                    result.push(component);
                }
                _ => {
                    result.push(component);
                }
            }
        }
        
        Ok(result)
    }
}

impl FileSystem for RealFileSystem {
    fn open(&self, path: &Path) -> UsmResult<Box<dyn Read + Send>> {
        let full_path = self.resolve_path(path);
        
        // Resolve symlinks before opening
        let resolved_path = match self.resolve_symlink_chain(&full_path) {
            Ok(resolved) => resolved,
            Err(_) => full_path, // Fallback to original path if symlink resolution fails
        };
        
        let file = std::fs::File::open(&resolved_path)?;
        
        // Check file size to prevent reading very large files
        let metadata = file.metadata()?;
        const MAX_FILE_SIZE: u64 = 1024 * 1024; // 1MB limit
        
        if metadata.len() > MAX_FILE_SIZE {
            return Err(UsmError::FileSystem(format!(
                "File too large: {} bytes",
                metadata.len()
            )));
        }
        
        Ok(Box::new(file))
    }
    
    fn open_seekable(&self, path: &Path) -> UsmResult<Box<dyn ReadSeek>> {
        let full_path = self.resolve_path(path);
        
        // Resolve symlinks before opening
        let resolved_path = match self.resolve_symlink_chain(&full_path) {
            Ok(resolved) => resolved,
            Err(_) => full_path, // Fallback to original path if symlink resolution fails
        };
        
        let file = std::fs::File::open(&resolved_path)?;
        
        // Check file size
        let metadata = file.metadata()?;
        const MAX_FILE_SIZE: u64 = 1024 * 1024; // 1MB limit
        
        if metadata.len() > MAX_FILE_SIZE {
            return Err(UsmError::FileSystem(format!(
                "File too large: {} bytes",
                metadata.len()
            )));
        }
        
        Ok(Box::new(file))
    }
    
    fn metadata(&self, path: &Path) -> UsmResult<FileInfo> {
        let full_path = self.resolve_path(path);
        let metadata = std::fs::metadata(&full_path)?;
        
        Ok(FileInfo {
            size: metadata.len(),
            is_dir: metadata.is_dir(),
            is_file: metadata.is_file(),
        })
    }
    
    fn exists(&self, path: &Path) -> bool {
        let full_path = self.resolve_path(path);
        full_path.exists()
    }
    
    fn read_dir(&self, path: &Path) -> UsmResult<Vec<PathBuf>> {
        let full_path = self.resolve_path(path);
        let entries = std::fs::read_dir(&full_path)?
            .collect::<Result<Vec<_>, io::Error>>()?;
        
        let mut paths = Vec::with_capacity(entries.len());
        for entry in entries {
            if let Some(name) = entry.file_name().to_str() {
                paths.push(PathBuf::from(name));
            }
        }
        
        paths.sort();
        Ok(paths)
    }
    
    fn read_link(&self, path: &Path) -> UsmResult<PathBuf> {
        let full_path = self.resolve_path(path);
        let target = std::fs::read_link(&full_path)?;
        Ok(target)
    }
    
    fn glob(&self, base: &Path, pattern: &str) -> UsmResult<Vec<PathBuf>> {
        let full_base = self.resolve_path(base);
        let _pattern_path = full_base.join(pattern);
        
        let mut matches = Vec::new();
        for entry in WalkDir::new(&full_base) {
            let entry = entry.map_err(|e| UsmError::FileSystem(e.to_string()))?;
            let path = entry.path();
            
            // Simple pattern matching (could be enhanced with proper glob)
            if pattern == "*" || pattern == "**/*" {
                if let Ok(relative) = path.strip_prefix(&full_base) {
                    matches.push(relative.to_path_buf());
                }
            } else if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                if pattern.contains('*') {
                    let pattern_regex = pattern.replace('*', ".*");
                    if regex::Regex::new(&pattern_regex)
                        .map_err(|e| UsmError::Parse(e.to_string()))?
                        .is_match(name)
                    {
                        if let Ok(relative) = path.strip_prefix(&full_base) {
                            matches.push(relative.to_path_buf());
                        }
                    }
                } else if name == pattern {
                    if let Ok(relative) = path.strip_prefix(&full_base) {
                        matches.push(relative.to_path_buf());
                    }
                }
            }
        }
        
        matches.sort();
        Ok(matches)
    }
    
    fn sub(&self, root: &Path) -> UsmResult<Box<dyn FileSystem>> {
        let new_root = self.resolve_path(root);
        Ok(Box::new(RealFileSystem::with_root(new_root)))
    }
    
    fn canonicalize(&self, path: &Path) -> UsmResult<PathBuf> {
        let full_path = self.resolve_path(path);
        
        // Use std::fs::canonicalize for complete path resolution
        match std::fs::canonicalize(&full_path) {
            Ok(canonical) => Ok(canonical),
            Err(e) => {
                // Fallback: manually resolve path components if canonicalize fails
                self.manual_canonicalize(&full_path).map_err(|_| UsmError::Io(e))
            }
        }
    }
    
    fn resolve_working_dir_relative(&self, path: &str, cwd: &Path) -> PathBuf {
        let path = Path::new(path);
        
        if path.is_absolute() {
            return path.to_path_buf();
        }
        
        // Resolve relative to working directory
        let mut resolved = cwd.to_path_buf();
        for component in path.components() {
            match component {
                std::path::Component::ParentDir => {
                    resolved.pop();
                }
                std::path::Component::CurDir => {
                    // Do nothing for "."
                }
                std::path::Component::Normal(part) => {
                    resolved.push(part);
                }
                _ => {
                    // Handle other component types as normal
                    resolved.push(component.as_os_str());
                }
            }
        }
        
        resolved
    }
    
    fn resolve_symlink_chain(&self, path: &Path) -> UsmResult<PathBuf> {
        let mut current = path.to_path_buf();
        let mut visited = std::collections::HashSet::new();
        const MAX_SYMLINK_DEPTH: usize = 40; // Prevent infinite loops
        let mut depth = 0;
        
        while depth < MAX_SYMLINK_DEPTH {
            // Check for loops
            if visited.contains(&current) {
                return Err(UsmError::FileSystem("Symlink loop detected".to_string()));
            }
            
            // Resolve current path
            let resolved_current = self.resolve_path(&current);
            
            // Check if it's a symlink
            match std::fs::symlink_metadata(&resolved_current) {
                Ok(metadata) if metadata.file_type().is_symlink() => {
                    visited.insert(current.clone());
                    
                    // Read the symlink target
                    match std::fs::read_link(&resolved_current) {
                        Ok(target) => {
                            // Handle relative symlink targets
                            if target.is_relative() {
                                if let Some(parent) = current.parent() {
                                    current = parent.join(target);
                                } else {
                                    current = target;
                                }
                            } else {
                                current = target;
                            }
                        }
                        Err(e) => {
                            return Err(UsmError::Io(e));
                        }
                    }
                }
                Ok(_) => {
                    // Not a symlink, we're done
                    return Ok(resolved_current);
                }
                Err(e) => {
                    return Err(UsmError::Io(e));
                }
            }
            
            depth += 1;
        }
        
        Err(UsmError::FileSystem("Maximum symlink depth exceeded".to_string()))
    }
}

/// In-memory filesystem for testing
#[derive(Debug, Default)]
pub struct MemoryFileSystem {
    files: HashMap<PathBuf, Vec<u8>>,
    dirs: HashMap<PathBuf, Vec<PathBuf>>,
}

impl MemoryFileSystem {
    pub fn new() -> Self {
        Self::default()
    }
    
    /// Add a file to the in-memory filesystem
    pub fn add_file<P: AsRef<Path>>(&mut self, path: P, content: Vec<u8>) {
        let path = path.as_ref().to_path_buf();
        
        // Add file
        self.files.insert(path.clone(), content);
        
        // Add to parent directory
        if let Some(parent) = path.parent() {
            let parent = parent.to_path_buf();
            self.dirs.entry(parent).or_default().push(path);
        }
    }
    
    /// Add a directory to the in-memory filesystem
    pub fn add_dir<P: AsRef<Path>>(&mut self, path: P) {
        let path = path.as_ref().to_path_buf();
        self.dirs.entry(path).or_default();
    }
}

impl FileSystem for MemoryFileSystem {
    fn open(&self, path: &Path) -> UsmResult<Box<dyn Read + Send>> {
        let content = self.files
            .get(path)
            .ok_or_else(|| UsmError::Io(io::Error::new(io::ErrorKind::NotFound, "File not found")))?;
        Ok(Box::new(io::Cursor::new(content.clone())))
    }
    
    fn open_seekable(&self, path: &Path) -> UsmResult<Box<dyn ReadSeek>> {
        let content = self.files
            .get(path)
            .ok_or_else(|| UsmError::Io(io::Error::new(io::ErrorKind::NotFound, "File not found")))?;
        Ok(Box::new(io::Cursor::new(content.clone())))
    }
    
    fn metadata(&self, path: &Path) -> UsmResult<FileInfo> {
        if let Some(content) = self.files.get(path) {
            Ok(FileInfo {
                size: content.len() as u64,
                is_dir: false,
                is_file: true,
            })
        } else if self.dirs.contains_key(path) {
            Ok(FileInfo {
                size: 0,
                is_dir: true,
                is_file: false,
            })
        } else {
            Err(UsmError::Io(io::Error::new(io::ErrorKind::NotFound, "Path not found")))
        }
    }
    
    fn exists(&self, path: &Path) -> bool {
        self.files.contains_key(path) || self.dirs.contains_key(path)
    }
    
    fn read_dir(&self, path: &Path) -> UsmResult<Vec<PathBuf>> {
        let entries = self.dirs
            .get(path)
            .ok_or_else(|| UsmError::Io(io::Error::new(io::ErrorKind::NotFound, "Directory not found")))?;
        
        let mut names = Vec::new();
        for entry in entries {
            if let Some(name) = entry.file_name() {
                names.push(PathBuf::from(name));
            }
        }
        names.sort();
        Ok(names)
    }
    
    fn read_link(&self, _path: &Path) -> UsmResult<PathBuf> {
        // Simplified implementation - no symlink support in memory FS
        Err(UsmError::FileSystem("Symlinks not supported in memory filesystem".to_string()))
    }
    
    fn glob(&self, base: &Path, pattern: &str) -> UsmResult<Vec<PathBuf>> {
        let mut matches = Vec::new();
        
        // Simple glob implementation
        for file_path in self.files.keys() {
            if let Ok(relative) = file_path.strip_prefix(base) {
                if pattern == "*" || pattern == "**/*" {
                    matches.push(relative.to_path_buf());
                } else if let Some(name) = relative.file_name().and_then(|n| n.to_str()) {
                    if pattern.contains('*') {
                        let pattern_regex = pattern.replace('*', ".*");
                        if regex::Regex::new(&pattern_regex)
                            .map_err(|e| UsmError::Parse(e.to_string()))?
                            .is_match(name)
                        {
                            matches.push(relative.to_path_buf());
                        }
                    } else if name == pattern {
                        matches.push(relative.to_path_buf());
                    }
                }
            }
        }
        
        matches.sort();
        Ok(matches)
    }
    
    fn sub(&self, _root: &Path) -> UsmResult<Box<dyn FileSystem>> {
        // For simplicity, return a clone
        Ok(Box::new(self.clone()))
    }
    
    fn canonicalize(&self, path: &Path) -> UsmResult<PathBuf> {
        // Simplified canonicalization for memory filesystem
        Ok(path.to_path_buf())
    }
    
    fn resolve_working_dir_relative(&self, path: &str, cwd: &Path) -> PathBuf {
        let path = Path::new(path);
        
        if path.is_absolute() {
            return path.to_path_buf();
        }
        
        cwd.join(path)
    }
    
    fn resolve_symlink_chain(&self, path: &Path) -> UsmResult<PathBuf> {
        // Memory filesystem doesn't support symlinks
        Ok(path.to_path_buf())
    }
}

impl Clone for MemoryFileSystem {
    fn clone(&self) -> Self {
        Self {
            files: self.files.clone(),
            dirs: self.dirs.clone(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Read;

    #[test]
    fn test_memory_filesystem() {
        let mut fs = MemoryFileSystem::new();
        fs.add_file("test.txt", b"hello world".to_vec());
        fs.add_dir("testdir");
        
        assert!(fs.exists(Path::new("test.txt")));
        assert!(fs.exists(Path::new("testdir")));
        assert!(!fs.exists(Path::new("nonexistent.txt")));
        
        let mut reader = fs.open(Path::new("test.txt")).unwrap();
        let mut content = String::new();
        reader.read_to_string(&mut content).unwrap();
        assert_eq!(content, "hello world");
        
        let info = fs.metadata(Path::new("test.txt")).unwrap();
        assert!(info.is_file);
        assert!(!info.is_dir);
        assert_eq!(info.size, 11);
    }
}