// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//! Erlang/BEAM service name detection.
//!
//! Mirrors the Go implementation: extracts -progname and -home flags from command-line arguments.

use crate::procfs::Cmdline;
use crate::service_name::{ServiceNameMetadata, ServiceNameSource};
use std::path::Path;

/// Extracts the Erlang app name from command-line arguments.
///
/// Matches Go semantics:
/// - If `-progname` is present and not "erl", return it
/// - Else if `-progname` is "erl" and `-home` is present, return basename of home
/// - Otherwise return None
pub fn extract_name(cmdline: &Cmdline) -> Option<ServiceNameMetadata> {
    let mut progname: Option<&str> = None;
    let mut home: Option<&str> = None;

    // Single pass over args, no allocation
    let mut args = cmdline.args();
    while let Some(arg) = args.next() {
        match arg {
            "-progname" => {
                if let Some(value) = args.next() {
                    progname = Some(value.trim());
                }
            }
            "-home" => {
                if let Some(value) = args.next() {
                    home = Some(value.trim());
                }
            }
            _ => continue,
        }
    }

    // Apply heuristics
    if let Some(p) = progname
        && !p.is_empty()
        && p != "erl"
    {
        return Some(ServiceNameMetadata::new(p, ServiceNameSource::CommandLine));
    }

    if progname == Some("erl")
        && let Some(h) = home
        && !h.is_empty()
    {
        let path = h.trim_end_matches('/');

        // Check if path ends with "/." or is exactly "." before Path normalization
        if path.ends_with("/.") || path == "." {
            return None;
        }

        let basename = Path::new(path).file_name()?.to_str()?;

        if !basename.is_empty() && basename != "." && basename != "/" {
            return Some(ServiceNameMetadata::new(
                basename,
                ServiceNameSource::CommandLine,
            ));
        }
    }

    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::cmdline;

    // Port of Test_detectErlangAppName from Go
    #[test]
    fn test_couchdb_with_progname() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-progname",
            "couchdb",
            "-home",
            "/opt/couchdb"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "couchdb",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_riak_with_progname() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-progname",
            "riak",
            "-home",
            "/var/lib/riak"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "riak",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_rabbitmq_with_erl_progname_use_home() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-progname",
            "erl",
            "-home",
            "/var/lib/rabbitmq"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "rabbitmq",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_ejabberd_with_erl_progname_use_home() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-progname",
            "erl",
            "-home",
            "/var/lib/ejabberd"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "ejabberd",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_generic_erlang_process_with_erl_progname_use_home() {
        let cmdline = cmdline!["beam.smp", "-progname", "erl", "-home", "/usr/local/myapp"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "myapp",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_no_progname_or_home_returns_none() {
        let cmdline = cmdline!["beam.smp", "-smp", "auto", "-noinput"];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_progname_without_home() {
        let cmdline = cmdline!["beam.smp", "-progname", "myerlangapp", "-smp", "auto"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "myerlangapp",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_empty_cmdline() {
        let cmdline = cmdline![];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_only_home_no_progname() {
        let cmdline = cmdline!["beam.smp", "-home", "/opt/customapp", "-noshell"];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_home_with_trailing_slash() {
        let cmdline = cmdline!["beam.smp", "-progname", "erl", "-home", "/var/lib/myapp/"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "myapp",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_complex_real_world_rabbitmq_command_line() {
        let cmdline = cmdline![
            "beam.smp",
            "-W",
            "w",
            "-MBas",
            "ageffcbf",
            "-MHas",
            "ageffcbf",
            "-MBlmbcs",
            "512",
            "-MHlmbcs",
            "512",
            "-MMmcs",
            "30",
            "-P",
            "1048576",
            "-t",
            "5000000",
            "-stbt",
            "db",
            "-zdbbl",
            "128000",
            "-sbwt",
            "none",
            "-sbwtdcpu",
            "none",
            "-sbwtdio",
            "none",
            "-K",
            "true",
            "-A",
            "192",
            "-sdio",
            "192",
            "-kernel",
            "inet_dist_listen_min",
            "25672",
            "-kernel",
            "inet_dist_listen_max",
            "25672",
            "-kernel",
            "shell_history",
            "enabled",
            "-boot",
            "/usr/lib/rabbitmq/bin/../releases/3.11.5/start_clean",
            "-lager",
            "crash_log",
            "false",
            "-lager",
            "handlers",
            "[]",
            "-rabbit",
            "product_name",
            "\"RabbitMQ\"",
            "-rabbit",
            "product_version",
            "\"3.11.5\"",
            "-progname",
            "erl",
            "-home",
            "/var/lib/rabbitmq"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "rabbitmq",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_couchdb_real_world_command_line() {
        let cmdline = cmdline![
            "beam.smp",
            "-noshell",
            "-noinput",
            "+Bd",
            "-B",
            "-K",
            "true",
            "-A",
            "16",
            "-n",
            "+A",
            "4",
            "+sbtu",
            "+sbwt",
            "none",
            "+sbwtdcpu",
            "none",
            "+sbwtdio",
            "none",
            "-config",
            "/opt/couchdb/releases/3.2.2/sys.config",
            "-sasl",
            "errlog_type",
            "error",
            "-couch_ini",
            "/opt/couchdb/etc/default.ini",
            "/opt/couchdb/etc/local.ini",
            "-boot",
            "/opt/couchdb/releases/3.2.2/couchdb",
            "-args_file",
            "/opt/couchdb/etc/vm.args",
            "-progname",
            "couchdb"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "couchdb",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_progname_with_spaces_in_value() {
        let cmdline = cmdline!["beam.smp", "-progname", "  couchdb  "];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "couchdb",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_home_with_spaces_in_value() {
        let cmdline = cmdline![
            "beam.smp",
            "-progname",
            "erl",
            "-home",
            "  /var/lib/rabbitmq  "
        ];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "rabbitmq",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_uppercase_erl_is_not_treated_as_erl() {
        let cmdline = cmdline!["beam.smp", "-progname", "ERL", "-home", "/var/lib/rabbitmq"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "ERL",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_mixed_case_erl_is_not_treated_as_erl() {
        let cmdline = cmdline!["beam.smp", "-progname", "Erl", "-home", "/var/lib/ejabberd"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "Erl",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_progname_as_last_argument_without_value() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-home",
            "/var/lib/myapp",
            "-progname"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_home_as_last_argument_without_value() {
        let cmdline = cmdline![
            "beam.smp",
            "-root",
            "/usr/lib/erlang",
            "-progname",
            "erl",
            "-home"
        ];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_home_basename_dot() {
        // '.' is not a valid basename (Go's filepath.Base(".") -> ".", which we then filter out)
        // This test validates that paths ending with "/." correctly return None
        let cmdline = cmdline!["beam.smp", "-progname", "erl", "-home", "/some/path/."];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    // Port of Test_erlangDetector from Go
    #[test]
    fn test_detector_couchdb() {
        let cmdline = cmdline!["beam.smp", "-progname", "couchdb", "-home", "/opt/couchdb"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "couchdb",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_detector_rabbitmq() {
        let cmdline = cmdline!["beam.smp", "-progname", "erl", "-home", "/var/lib/rabbitmq"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "rabbitmq",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_detector_no_name_extracted() {
        let cmdline = cmdline!["beam.smp", "-smp", "auto"];
        let result = extract_name(&cmdline);
        assert_eq!(result, None);
    }

    #[test]
    fn test_detector_custom_erlang_app() {
        let cmdline = cmdline!["beam.smp", "-progname", "myapp", "-noshell"];
        let result = extract_name(&cmdline);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "myapp",
                ServiceNameSource::CommandLine,
            ))
        );
    }
}
