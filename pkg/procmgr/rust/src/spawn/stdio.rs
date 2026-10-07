// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use std::path::PathBuf;

#[derive(Debug, PartialEq, Eq)]
pub(crate) enum StdioSetting {
    Inherit,
    Null,
    File(PathBuf),
}

pub(crate) fn parse_stdio_setting(yaml_value: &str) -> StdioSetting {
    match yaml_value {
        "null" => StdioSetting::Null,
        "inherit" | "" => StdioSetting::Inherit,
        path => StdioSetting::File(PathBuf::from(path)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_stdio_setting_maps_keywords_and_paths() {
        assert_eq!(parse_stdio_setting("inherit"), StdioSetting::Inherit);
        assert_eq!(parse_stdio_setting(""), StdioSetting::Inherit);
        assert_eq!(parse_stdio_setting("null"), StdioSetting::Null);
        assert_eq!(
            parse_stdio_setting(r"C:\logs\out.log"),
            StdioSetting::File(PathBuf::from(r"C:\logs\out.log"))
        );
    }
}
