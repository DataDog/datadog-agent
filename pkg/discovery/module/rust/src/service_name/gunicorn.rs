// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! This package implements Gunicorn service name generation.

use std::collections::HashMap;

use crate::procfs::Cmdline;
use crate::service_name::{ServiceNameMetadata, ServiceNameSource};
use phf::phf_set;

const GUNICORN_CMD_ARGS: &str = "GUNICORN_CMD_ARGS";
const WSGI_APP_ENV: &str = "WSGI_APP";

pub fn extract_name(cmdline: &Cmdline, envs: &HashMap<String, String>) -> ServiceNameMetadata {
    extract_name_from_args(cmdline.args().skip(1), envs)
}

pub fn extract_name_from_args<'a>(
    args: impl Iterator<Item = &'a str>,
    envs: &HashMap<String, String>,
) -> ServiceNameMetadata {
    // First check GUNICORN_CMD_ARGS environment variable
    if let Some(env_args) = envs.get(GUNICORN_CMD_ARGS)
        && let Some(name) = extract_gunicorn_name_from(env_args.split_whitespace())
    {
        return ServiceNameMetadata::new(name, ServiceNameSource::Gunicorn);
    }

    // Check WSGI_APP environment variable
    if let Some(wsgi_app) = envs.get(WSGI_APP_ENV)
        && !wsgi_app.is_empty()
    {
        return ServiceNameMetadata::new(
            parse_name_from_wsgi_app(wsgi_app),
            ServiceNameSource::Gunicorn,
        );
    }

    // Parse command line arguments
    if let Some(name) = extract_gunicorn_name_from(args) {
        return ServiceNameMetadata::new(name, ServiceNameSource::CommandLine);
    }

    // Default to "gunicorn" if no name found
    ServiceNameMetadata::new("gunicorn", ServiceNameSource::CommandLine)
}

fn extract_gunicorn_name_from<'a>(mut args: impl Iterator<Item = &'a str>) -> Option<String> {
    // Set of long options that do NOT take an argument
    static NO_ARG_OPTIONS: phf::Set<&'static str> = phf_set! {
        "--reload",
        "--spew",
        "--check-config",
        "--print-config",
        "--preload",
        "--no-sendfile",
        "--reuse-port",
        "--daemon",
        "--initgroups",
        "--capture-output",
        "--log-syslog",
        "--enable-stdio-inheritance",
        "--disable-redirect-access-to-syslog",
        "--proxy-protocol",
        "--suppress-ragged-eofs",
        "--do-handshake-on-connect",
        "--strip-header-spaces",
    };

    let mut candidate: Option<&'a str> = None;
    let mut last_arg: Option<&'a str> = None;

    while let Some(arg) = args.next() {
        last_arg = Some(arg);

        // --name=foo
        if let Some(value) = arg.strip_prefix("--name=") {
            return Some(value.to_string());
        }

        // --name foo
        if arg == "--name" {
            if let Some(name) = args.next() {
                return Some(name.to_string());
            }
            break;
        }

        // Long options
        if let Some(rest) = arg.strip_prefix("--") {
            // --option=value
            if rest.contains('=') {
                continue;
            }
            // --option value
            if !NO_ARG_OPTIONS.contains(arg)
                && let Some(next) = args.next()
            {
                last_arg = Some(next);
            }
            continue;
        }

        // Short / grouped flags
        if let Some(flags) = arg.strip_prefix('-') {
            let mut chars = flags.chars();
            while let Some(c) = chars.next() {
                match c {
                    'n' => {
                        // -nfoo
                        let rest: String = chars.collect();
                        if !rest.is_empty() {
                            return Some(rest);
                        }
                        // -n foo
                        if let Some(name) = args.next() {
                            return Some(name.to_string());
                        }
                        break;
                    }
                    'R' | 'd' => {
                        continue;
                    }
                    _ => {
                        let rest: String = chars.collect();
                        if !rest.is_empty() {
                            break;
                        }
                        // -x foo
                        if let Some(next) = args.next() {
                            last_arg = Some(next);
                        }
                        break;
                    }
                }
            }
            continue;
        }
        // module:app pattern
        if candidate.is_none() {
            candidate = Some(arg);
        }
    }

    if let Some(last) = last_arg
        && let Some(inner) = last.strip_prefix('[').and_then(|s| s.strip_suffix(']'))
    {
        return Some(parse_name_from_wsgi_app(inner));
    }

    if let Some(c) = candidate {
        return Some(parse_name_from_wsgi_app(c));
    }

    None
}

fn parse_name_from_wsgi_app(wsgi_app: &str) -> String {
    wsgi_app.split(':').next().unwrap_or(wsgi_app).to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_gunicorn_name_from() {
        let tests = vec![
            (
                "simple module:app pattern",
                vec!["myapp:app"],
                Some("myapp"),
            ),
            (
                "module:app with parentheses",
                vec!["FOOd:create_app()"],
                Some("FOOd"),
            ),
            (
                "module:app with complex arguments",
                vec![
                    "foo.middleware:make_app(config_path='/foo/web.ini')",
                    "--bind",
                    "0.0.0.0:5555",
                    "--workers",
                    "4",
                    "--bind",
                    "127.0.0.1:5123",
                    "--workers",
                    "4",
                    "--worker-class",
                    "eventlet",
                    "--keep-alive",
                    "300",
                    "--timeout",
                    "120",
                    "--graceful-timeout",
                    "60",
                    "--log-config",
                    "/etc/log.ini",
                    "--statsd-host",
                    "10.2.1.1:8125",
                    "--statsd-prefix",
                    "foo.web",
                ],
                Some("foo.middleware"),
            ),
            (
                "app name not last argument",
                vec!["myapp:app", "--bind", "0.0.0.0:8000", "--workers", "1"],
                Some("myapp"),
            ),
            (
                "app name after no arg flag",
                vec!["--worker-class", "foo", "--daemon", "myapp"],
                Some("myapp"),
            ),
            ("with name flag", vec!["-n", "myapp"], Some("myapp")),
            (
                "with name flag and value",
                vec!["--name=myapp"],
                Some("myapp"),
            ),
            (
                "with name flag separated by space",
                vec!["--name", "myapp"],
                Some("myapp"),
            ),
            (
                "with name flag separated by space and other args",
                vec![
                    "--bind",
                    "0.0.0.0:8000",
                    "--name",
                    "myapp",
                    "--workers",
                    "4",
                ],
                Some("myapp"),
            ),
            (
                "with name flag and attached value",
                vec!["-nmyapp"],
                Some("myapp"),
            ),
            (
                "with name flag and attached value with other flag",
                vec!["-Rnmyapp"],
                Some("myapp"),
            ),
            ("with other flag", vec!["-unfake", "myapp"], Some("myapp")),
            (
                "with name flag and other flags",
                vec!["-R", "-nmyapp", "--bind", "0.0.0.0:8000"],
                Some("myapp"),
            ),
            (
                "with name flag and other flags reversed",
                vec!["--bind", "0.0.0.0:8000", "-nmyapp", "-R"],
                Some("myapp"),
            ),
            (
                "with name flag and module:app pattern",
                vec!["-nmyapp", "test:app"],
                Some("myapp"),
            ),
            (
                "with name flag and module:app pattern reversed",
                vec!["test:app", "-nmyapp"],
                Some("myapp"),
            ),
            ("no app name found", vec!["--bind", "0.0.0.0:8000"], None),
        ];

        for (name, args, expected) in tests {
            let result = extract_gunicorn_name_from(args.into_iter());
            assert_eq!(result.as_deref(), expected, "Test case '{}' failed", name);
        }
    }

    #[test]
    fn test_parse_name_from_wsgi_app() {
        assert_eq!(parse_name_from_wsgi_app("myapp:app"), "myapp");
        assert_eq!(parse_name_from_wsgi_app("foo.bar:create_app()"), "foo.bar");
        assert_eq!(parse_name_from_wsgi_app("myapp"), "myapp");
    }

    #[test]
    fn test_extract_gunicorn_name_from_edge_cases() {
        let tests = vec![
            // Empty and whitespace cases
            ("empty args", vec![], None),
            ("only flags no app", vec!["--bind", "0.0.0.0:8000"], None),
            // Bracket edge cases
            (
                "single opening bracket",
                vec!["[incomplete"],
                Some("[incomplete"),
            ),
            (
                "single closing bracket",
                vec!["incomplete]"],
                Some("incomplete]"),
            ),
            ("empty brackets", vec!["[]"], Some("")),
            ("brackets with spaces", vec!["[ spaced ]"], Some(" spaced ")),
            (
                "multiple bracketed args - last wins",
                vec!["[first]", "[second]", "[third]"],
                Some("third"),
            ),
            // Flag edge cases
            ("name flag without value", vec!["-n"], None),
            ("long name flag without value", vec!["--name"], None),
            ("name flag equals empty", vec!["--name="], Some("")),
            (
                "grouped flags ending with n but no value",
                vec!["-Rdn"],
                None,
            ),
            // WSGI app edge cases
            (
                "multiple colons in wsgi app",
                vec!["app:factory:extra"],
                Some("app"),
            ),
            ("colon only", vec![":"], Some("")),
            ("ending with colon", vec!["app:"], Some("app")),
            // Special characters
            (
                "unicode in app name",
                vec!["myapp-café:app"],
                Some("myapp-café"),
            ),
            (
                "dashes and underscores",
                vec!["my-app_name:app"],
                Some("my-app_name"),
            ),
            // Mixed scenarios
            (
                "bracket after name flag should use flag",
                vec!["-nmyapp", "[ignored]"],
                Some("myapp"),
            ),
            (
                "flag consumes next arg then app",
                vec!["-x", "consumed", "myapp:app"],
                Some("myapp"),
            ),
        ];

        for (name, args, expected) in tests {
            let result = extract_gunicorn_name_from(args.into_iter());
            assert_eq!(result.as_deref(), expected, "Test case '{}' failed", name);
        }
    }

    #[test]
    fn test_parse_name_from_wsgi_app_edge_cases() {
        // Empty and special cases
        assert_eq!(parse_name_from_wsgi_app(""), "");
        assert_eq!(parse_name_from_wsgi_app(":"), "");
        assert_eq!(parse_name_from_wsgi_app("::"), "");
        assert_eq!(parse_name_from_wsgi_app("app:"), "app");
        assert_eq!(parse_name_from_wsgi_app(":app"), "");

        // Multiple colons
        assert_eq!(parse_name_from_wsgi_app("a:b:c:d"), "a");

        // Unicode and special characters
        assert_eq!(parse_name_from_wsgi_app("café:app"), "café");
        assert_eq!(parse_name_from_wsgi_app("my-app_v2:create"), "my-app_v2");
    }

    #[test]
    fn test_extract_name_with_environment_variables() {
        let cmdline = Cmdline::from(&["gunicorn", "default:app"][..]);
        let mut envs = HashMap::new();

        // GUNICORN_CMD_ARGS takes precedence over cmdline
        envs.insert(GUNICORN_CMD_ARGS.to_string(), "-n envname".to_string());
        let result = extract_name(&cmdline, &envs);
        assert_eq!(result.name, "envname");
        assert_eq!(result.source, ServiceNameSource::Gunicorn);

        // WSGI_APP is used if GUNICORN_CMD_ARGS doesn't have a name
        let mut envs2 = HashMap::new();
        envs2.insert(
            GUNICORN_CMD_ARGS.to_string(),
            "--bind 0.0.0.0:8000".to_string(),
        );
        envs2.insert(WSGI_APP_ENV.to_string(), "wsgi:app".to_string());
        let result2 = extract_name(&cmdline, &envs2);
        assert_eq!(result2.name, "wsgi");
        assert_eq!(result2.source, ServiceNameSource::Gunicorn);

        // Empty WSGI_APP falls back to cmdline
        let mut envs3 = HashMap::new();
        envs3.insert(WSGI_APP_ENV.to_string(), "".to_string());
        let result3 = extract_name(&cmdline, &envs3);
        assert_eq!(result3.name, "default");
        assert_eq!(result3.source, ServiceNameSource::CommandLine);

        // No env vars, uses cmdline
        let envs4 = HashMap::new();
        let result4 = extract_name(&cmdline, &envs4);
        assert_eq!(result4.name, "default");
        assert_eq!(result4.source, ServiceNameSource::CommandLine);

        // No app in cmdline or env, defaults to "gunicorn"
        let cmdline_no_app = Cmdline::from(&["gunicorn", "--bind", "0.0.0.0:8000"][..]);
        let result5 = extract_name(&cmdline_no_app, &HashMap::new());
        assert_eq!(result5.name, "gunicorn");
        assert_eq!(result5.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_name_flag_short() {
        let cmdline = Cmdline::from(
            &[
                "gunicorn",
                "--workers=2",
                "-b",
                "0.0.0.0",
                "-n",
                "dummy",
                "test:app",
            ][..],
        );
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "dummy");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_name_flag_long() {
        let cmdline = Cmdline::from(
            &[
                "gunicorn",
                "--workers=2",
                "-b",
                "0.0.0.0",
                "--name=dummy",
                "test:app",
            ][..],
        );
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "dummy");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_env_name() {
        let mut envs = HashMap::new();
        envs.insert(
            "GUNICORN_CMD_ARGS".to_string(),
            "--bind=127.0.0.1:8080 --workers=3 -n dummy".to_string(),
        );
        let cmdline = Cmdline::from(&["gunicorn", "test:app"][..]);
        let result = extract_name(&cmdline, &envs);
        assert_eq!(result.name, "dummy");
        assert_eq!(result.source, ServiceNameSource::Gunicorn);
    }

    #[test]
    fn test_extract_name_without_app_defaults_to_gunicorn() {
        let mut envs = HashMap::new();
        envs.insert(
            "GUNICORN_CMD_ARGS".to_string(),
            "--bind=127.0.0.1:8080 --workers=3".to_string(),
        );
        let cmdline = Cmdline::from(&["gunicorn"][..]);
        let result = extract_name(&cmdline, &envs);
        assert_eq!(result.name, "gunicorn");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_partial_wsgi_app() {
        let cmdline = Cmdline::from(&["gunicorn", "my.package"][..]);
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "my.package");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_empty_wsgi_app_env() {
        let mut envs = HashMap::new();
        envs.insert("WSGI_APP".to_string(), "".to_string());
        let cmdline = Cmdline::from(&["gunicorn", "my.package"][..]);
        let result = extract_name(&cmdline, &envs);
        assert_eq!(result.name, "my.package");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_wsgi_app_env() {
        let mut envs = HashMap::new();
        envs.insert("WSGI_APP".to_string(), "test:app".to_string());
        let cmdline = Cmdline::from(&["gunicorn"][..]);
        let result = extract_name(&cmdline, &envs);
        assert_eq!(result.name, "test");
        assert_eq!(result.source, ServiceNameSource::Gunicorn);
    }

    #[test]
    fn test_extract_name_with_bracketed_name_with_colon() {
        let cmdline = Cmdline::from(
            &[
                "gunicorn:",
                "master",
                "[domains.foo.apps.bar:create_server()]",
            ][..],
        );
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "domains.foo.apps.bar");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_bracketed_name() {
        let cmdline = Cmdline::from(&["gunicorn:", "master", "[mcservice]"][..]);
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "mcservice");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_ready_prefix() {
        let cmdline = Cmdline::from(&["[ready]", "gunicorn:", "worker", "[airflow-webserver]"][..]);
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "airflow-webserver");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_long_name_flag_separated() {
        let cmdline = Cmdline::from(
            &[
                "gunicorn",
                "--workers=2",
                "-b",
                "0.0.0.0",
                "--name",
                "my-service",
                "test:app",
            ][..],
        );
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "my-service");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_extract_name_with_long_name_flag_no_value() {
        let cmdline = Cmdline::from(&["gunicorn", "--name"][..]);
        let result = extract_name(&cmdline, &HashMap::new());
        assert_eq!(result.name, "gunicorn");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }
}
