// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! This package implements Uvicorn service name generation.

use crate::service_name::{ServiceNameMetadata, ServiceNameSource};

pub fn extract_name_from_args<'a>(
    mut args: impl Iterator<Item = &'a str>,
) -> Option<ServiceNameMetadata> {
    while let Some(arg) = args.next() {
        // Skip flags
        if arg.starts_with('-') {
            if arg == "--header" {
                // This takes an argument which looks similar to a module:app
                // pattern so skip that
                args.next();
            }
            continue;
        }

        // Look for module:app pattern
        if let Some((module, _)) = arg.split_once(':') {
            return Some(ServiceNameMetadata::new(
                module,
                ServiceNameSource::CommandLine,
            ));
        }
    }

    Some(ServiceNameMetadata::new(
        "uvicorn",
        ServiceNameSource::CommandLine,
    ))
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {
    use super::*;
    use crate::service_name::ServiceNameSource;

    #[test]
    fn test_uvicorn_with_first_arg() {
        let args = vec!["myapp.asgi:application", "--host=0.0.0.0", "--port=8000"];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "myapp.asgi");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_uvicorn_with_middle_args() {
        let args = vec![
            "--factory",
            "--host=0.0.0.0",
            "--port=8000",
            "app:create_app",
            "--workers=4",
        ];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "app");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_uvicorn_with_header() {
        let args = vec!["--header=X-Foo:Bar", "api.v1.app:app"];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "api.v1.app");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_uvicorn_with_header_separate() {
        let args = vec!["--header", "X-Foo:Bar", "api.v1.app:app"];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "api.v1.app");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_uvicorn_with_header_separate_last() {
        let args = vec!["api.v1.app:app", "--header", "X-Foo:Bar"];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "api.v1.app");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_uvicorn_unknown() {
        let args = vec!["foo"];
        let result = extract_name_from_args(args.into_iter()).unwrap();
        assert_eq!(result.name, "uvicorn");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }
}
