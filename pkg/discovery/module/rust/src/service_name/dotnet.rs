// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use crate::service_name::{ServiceNameMetadata, ServiceNameSource};
use std::path::Path;

const DLL_EXTENSION: &str = ".dll";

pub fn extract_name(cmdline: &crate::procfs::Cmdline) -> Option<ServiceNameMetadata> {
    let mut args = cmdline.args();

    // Skip the first arg (dotnet executable)
    args.next()?;

    for arg in args {
        // Skip flags (arguments starting with -)
        if arg.starts_with('-') {
            continue;
        }

        // Check if this is a .dll file (case-insensitive)
        if arg.to_lowercase().ends_with(DLL_EXTENSION) {
            // Extract filename from path
            let path = Path::new(arg);
            let filename = path.file_name()?.to_str()?;

            // Remove .dll extension
            let name = filename.get(..filename.len() - DLL_EXTENSION.len())?;
            if name.is_empty() {
                return None;
            }

            return Some(ServiceNameMetadata::new(
                name,
                ServiceNameSource::CommandLine,
            ));
        }

        // dotnet cli syntax is something like dotnet <cmd> <args> <dll> <prog
        // args> if the first non arg (-v, --something, ...) is not a dll file,
        // exit early since nothing matches a dll execute case
        break;
    }

    None
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::cmdline;
    use crate::procfs::Cmdline;

    #[test]
    fn test_dotnet_simple_dll() {
        let cmdline = cmdline!["/usr/bin/dotnet", "./myservice.dll"];

        let result = extract_name(&cmdline);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myservice");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_dotnet_dll_with_flags() {
        let cmdline = cmdline!["/usr/bin/dotnet", "-v", "--", "/app/lib/myservice.dll"];

        let result = extract_name(&cmdline);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "myservice");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_dotnet_dll_case_insensitive() {
        let cmdline = cmdline!["/usr/bin/dotnet", "MyService.DLL"];

        let result = extract_name(&cmdline);

        assert!(result.is_some());
        let metadata = result.unwrap();
        assert_eq!(metadata.name, "MyService");
        assert_eq!(metadata.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_dotnet_no_dll_file() {
        let cmdline = cmdline![
            "/usr/bin/dotnet",
            "run",
            "--project",
            "./projects/proj1/proj1.csproj"
        ];

        let result = extract_name(&cmdline);

        assert!(result.is_none());
    }

    #[test]
    fn test_dotnet_empty_cmdline() {
        let cmdline = cmdline!["/usr/bin/dotnet"];

        let result = extract_name(&cmdline);

        assert!(result.is_none());
    }

    #[test]
    fn test_dotnet_malformed_cmdline() {
        let cmdline = cmdline!["/usr/bin/dotnet", ".dll"];

        let result = extract_name(&cmdline);

        assert!(result.is_none());
    }

    #[test]
    fn test_dotnet_dll_after_non_dll_arg() {
        // First non-flag arg is not a dll, so we should stop searching
        let cmdline = cmdline!["/usr/bin/dotnet", "run", "myservice.dll"];

        let result = extract_name(&cmdline);

        assert!(result.is_none());
    }
}
