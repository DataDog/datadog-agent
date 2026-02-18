// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

/// Extract the longest path prefix without wildcards
pub fn longest_path_prefix(pattern: &str) -> &str {
    if let Some((prefix, _)) = pattern.split_once(['?', '*']) {
        return prefix
            .rsplit_once('/')
            .map(|(prefix, _)| prefix)
            .unwrap_or("");
    };

    pattern
}

/// Match a path against an ant-style pattern
/// Uses glob-match which supports **, *, ?, [ab], {a,b} patterns
pub fn matches(pattern: &str, path: &str) -> bool {
    glob_match::glob_match(pattern, path)
}

/// Check if a path could potentially match a pattern
/// This is used to skip entire directory trees during traversal
/// Returns true if the path matches OR if it's a prefix that could lead to matches
pub fn match_start(pattern: &str, path: &str) -> bool {
    // If it already matches, return true
    if matches(pattern, path) {
        return true;
    }

    // Check if this path is a potential prefix for matches
    let prefix = longest_path_prefix(pattern);

    // If the pattern has no prefix (e.g., "*.xml"), we need to explore everything
    if prefix.is_empty() {
        return true;
    }

    // Check if path is on the way to where matches could occur
    // Either path starts with the pattern's prefix, or the pattern's prefix starts with path
    path.starts_with(prefix) || prefix.starts_with(path)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_longest_path_prefix() {
        assert_eq!(longest_path_prefix("/test/**/*.xml"), "/test");
        assert_eq!(longest_path_prefix("/test/foo.xml"), "/test/foo.xml");
        assert_eq!(longest_path_prefix("**/*.xml"), "");
        assert_eq!(longest_path_prefix("/test/*/foo.xml"), "/test");
    }

    #[test]
    fn test_match_start() {
        // Pattern: /test/**/*.xml
        assert!(match_start("/test/**/*.xml", "/test"));
        assert!(match_start("/test/**/*.xml", "/test/foo"));
        assert!(match_start("/test/**/*.xml", "/test/foo/bar.xml"));
        assert!(!match_start("/test/**/*.xml", "/other"));

        // Pattern: testdata/*/application.properties
        assert!(match_start("testdata/*/application.properties", "testdata"));
        assert!(match_start(
            "testdata/*/application.properties",
            "testdata/subfolder"
        ));
        assert!(match_start(
            "testdata/*/application.properties",
            "testdata/subfolder/application.properties"
        ));
        assert!(!match_start("testdata/*/application.properties", "other"));

        // Pattern with no prefix (**.xml) - should match everything
        assert!(match_start("**/*.xml", "any"));
        assert!(match_start("**/*.xml", "any/path"));
        assert!(match_start("*.xml", "foo.xml"));
    }
}
