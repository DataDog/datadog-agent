use serde::{Deserialize, Serialize};

/// Programming language detected for a process
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum Language {
    /// Unknown or unsupported language
    Unknown,
    /// Java (including Scala, Kotlin on JVM)
    Java,
    /// Python
    Python,
    /// Node.js/JavaScript
    Node,
    /// PHP
    PHP,
    /// Ruby
    Ruby,
    /// .NET (C#, F#, VB.NET)
    DotNet,
    /// Go
    Go,
    /// Rust
    Rust,
    /// C/C++
    Cpp,
}

impl Language {
    /// Returns the string representation of the language
    pub fn as_str(&self) -> &'static str {
        match self {
            Language::Unknown => "unknown",
            Language::Java => "java",
            Language::Python => "python",
            Language::Node => "node",
            Language::PHP => "php",
            Language::Ruby => "ruby",
            Language::DotNet => "dotnet",
            Language::Go => "go",
            Language::Rust => "rust",
            Language::Cpp => "cpp",
        }
    }

    /// Parse language from string
    pub fn from_str(s: &str) -> Self {
        match s.to_lowercase().as_str() {
            "java" => Language::Java,
            "python" => Language::Python,
            "node" | "nodejs" | "javascript" | "js" => Language::Node,
            "php" => Language::PHP,
            "ruby" => Language::Ruby,
            "dotnet" | ".net" | "csharp" | "c#" => Language::DotNet,
            "go" | "golang" => Language::Go,
            "rust" => Language::Rust,
            "cpp" | "c++" | "c" => Language::Cpp,
            _ => Language::Unknown,
        }
    }
}

impl std::fmt::Display for Language {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

impl Default for Language {
    fn default() -> Self {
        Language::Unknown
    }
}