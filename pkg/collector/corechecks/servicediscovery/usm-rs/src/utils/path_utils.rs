use std::path::{Path, PathBuf};

/// Remove file path and return just the filename
pub fn remove_file_path(s: &str) -> String {
    if s.is_empty() {
        return s.to_string();
    }
    
    let path = Path::new(s);
    path.file_name()
        .and_then(|name| name.to_str())
        .unwrap_or(s)
        .to_string()
}

/// Trim colon and everything to the right of it
pub fn trim_colon_right(s: &str) -> String {
    if let Some(colon_pos) = s.find(':') {
        if colon_pos > 0 {
            return s[..colon_pos].to_string();
        }
    }
    s.to_string()
}

/// Check if the character at the given position is a letter
pub fn is_rune_letter_at(s: &str, position: usize) -> bool {
    s.chars()
        .nth(position)
        .map_or(false, |c| c.is_alphabetic())
}

/// Parse executable names that start with special characters like "(", "-", or "["
pub fn parse_exe_start_with_symbol(exe: &str) -> String {
    if exe.is_empty() {
        return exe.to_string();
    }
    
    let chars: Vec<char> = exe.chars().collect();
    if chars.is_empty() {
        return exe.to_string();
    }
    
    // Drop the first character
    let mut result = if chars.len() > 1 {
        chars[1..].iter().collect::<String>()
    } else {
        String::new()
    };
    
    // If the last character is also a special character, drop it
    if !result.is_empty() {
        let last_char = result.chars().last().unwrap();
        if !last_char.is_alphabetic() {
            result.pop();
        }
    }
    
    result
}

/// Validate version string (should contain only numbers and dots)
pub fn valid_version(s: &str) -> bool {
    if s.is_empty() {
        return true;
    }
    
    for part in s.split('.') {
        if part.is_empty() {
            return false;
        }
        for c in part.chars() {
            if !c.is_ascii_digit() {
                return false;
            }
        }
    }
    true
}

/// Normalize executable name (handle versioned executables like php7.4)
pub fn normalize_exe_name(exe: &str) -> String {
    // Handle PHP executable with version number - phpX.X
    if exe.starts_with("php") {
        let suffix = &exe[3..];
        if valid_version(suffix) {
            return "php".to_string();
        }
    }
    
    // Handle other versioned executables
    // python3.9 -> python
    if exe.starts_with("python") {
        let suffix = &exe[6..];
        if suffix.is_empty() || valid_version(suffix) {
            return "python".to_string();
        }
    }
    
    // node18 -> node
    if exe.starts_with("node") {
        let suffix = &exe[4..];
        if suffix.is_empty() || suffix.chars().all(|c| c.is_ascii_digit()) {
            return "node".to_string();
        }
    }
    
    exe.to_string()
}

/// Convert relative path to absolute by joining with current working directory
pub fn abs(path: &str, cwd: &str) -> String {
    let path = Path::new(path);
    let cwd = Path::new(cwd);
    
    if path.is_absolute() || cwd.as_os_str().is_empty() {
        path.to_string_lossy().to_string()
    } else {
        cwd.join(path).to_string_lossy().to_string()
    }
}

/// Check if a file has a JavaScript extension
pub fn is_js_file(filepath: &str) -> bool {
    let path = Path::new(filepath);
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        matches!(ext.to_lowercase().as_str(), "js" | "mjs" | "cjs")
    } else {
        false
    }
}

/// Check if a file has a Python extension
pub fn is_python_file(filepath: &str) -> bool {
    let path = Path::new(filepath);
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        matches!(ext.to_lowercase().as_str(), "py" | "pyw")
    } else {
        false
    }
}

/// Check if a file has a Java archive extension
pub fn is_java_archive(filepath: &str) -> bool {
    let path = Path::new(filepath);
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        matches!(ext.to_lowercase().as_str(), "jar" | "war" | "ear")
    } else {
        false
    }
}

/// Check if a file has a .NET assembly extension
pub fn is_dotnet_assembly(filepath: &str) -> bool {
    let path = Path::new(filepath);
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        matches!(ext.to_lowercase().as_str(), "dll" | "exe")
    } else {
        false
    }
}

/// Extract file extension
pub fn get_file_extension(filepath: &str) -> Option<String> {
    Path::new(filepath)
        .extension()
        .and_then(|ext| ext.to_str())
        .map(|ext| ext.to_lowercase())
}

/// Remove file extension from filename
pub fn remove_file_extension(filename: &str) -> String {
    let path = Path::new(filename);
    if let Some(stem) = path.file_stem().and_then(|s| s.to_str()) {
        stem.to_string()
    } else {
        filename.to_string()
    }
}

/// Join path components
pub fn join_paths(base: &str, components: &[&str]) -> String {
    let mut path = PathBuf::from(base);
    for component in components {
        path = path.join(component);
    }
    path.to_string_lossy().to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_remove_file_path() {
        assert_eq!(remove_file_path("/usr/bin/java"), "java");
        assert_eq!(remove_file_path("java"), "java");
        assert_eq!(remove_file_path(""), "");
        assert_eq!(remove_file_path("/path/to/app.jar"), "app.jar");
    }

    #[test]
    fn test_trim_colon_right() {
        assert_eq!(trim_colon_right("java:8"), "java");
        assert_eq!(trim_colon_right("java"), "java");
        assert_eq!(trim_colon_right(":test"), ":test"); // No trim if colon at start
        assert_eq!(trim_colon_right(""), "");
    }

    #[test]
    fn test_is_rune_letter_at() {
        assert!(is_rune_letter_at("java", 0));
        assert!(!is_rune_letter_at("8java", 0));
        assert!(is_rune_letter_at("8java", 1));
        assert!(!is_rune_letter_at("", 0));
        assert!(!is_rune_letter_at("java", 10));
    }

    #[test]
    fn test_parse_exe_start_with_symbol() {
        assert_eq!(parse_exe_start_with_symbol("(java)"), "java");
        assert_eq!(parse_exe_start_with_symbol("[python]"), "python");
        assert_eq!(parse_exe_start_with_symbol("-node"), "node");
        assert_eq!(parse_exe_start_with_symbol("(test"), "test");
        assert_eq!(parse_exe_start_with_symbol(""), "");
        assert_eq!(parse_exe_start_with_symbol("a"), "");
    }

    #[test]
    fn test_valid_version() {
        assert!(valid_version(""));
        assert!(valid_version("1"));
        assert!(valid_version("1.2"));
        assert!(valid_version("1.2.3"));
        assert!(!valid_version("1.2.a"));
        assert!(!valid_version("a.b.c"));
        assert!(!valid_version("1..2"));
    }

    #[test]
    fn test_normalize_exe_name() {
        assert_eq!(normalize_exe_name("php"), "php");
        assert_eq!(normalize_exe_name("php7.4"), "php");
        assert_eq!(normalize_exe_name("php8.1"), "php");
        assert_eq!(normalize_exe_name("phpunit"), "phpunit"); // Not normalized
        assert_eq!(normalize_exe_name("python3.9"), "python");
        assert_eq!(normalize_exe_name("python"), "python");
        assert_eq!(normalize_exe_name("node18"), "node");
        assert_eq!(normalize_exe_name("java"), "java");
    }

    #[test]
    fn test_file_type_checks() {
        assert!(is_js_file("app.js"));
        assert!(is_js_file("module.mjs"));
        assert!(is_js_file("script.cjs"));
        assert!(!is_js_file("app.py"));

        assert!(is_python_file("script.py"));
        assert!(is_python_file("gui.pyw"));
        assert!(!is_python_file("script.js"));

        assert!(is_java_archive("app.jar"));
        assert!(is_java_archive("webapp.war"));
        assert!(is_java_archive("enterprise.ear"));
        assert!(!is_java_archive("app.py"));

        assert!(is_dotnet_assembly("app.dll"));
        assert!(is_dotnet_assembly("program.exe"));
        assert!(!is_dotnet_assembly("script.py"));
    }
}