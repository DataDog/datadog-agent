// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::path::Path;

use crate::{
    procfs::Cmdline,
    service_name::{ServiceNameMetadata, ServiceNameSource},
};

pub fn extract_name(cmdline: &Cmdline) -> Option<ServiceNameMetadata> {
    let mut args = cmdline.args();

    // Skip executable (first arg)
    args.next()?;

    while let Some(arg) = args.next() {
        // The Go implementation says that this check is to "skip environment
        // variables", but it's unclear what that means since ruby doesn't allow
        // environment variables in the command line to the ruby executable
        // (ruby FOO=true foo.rb).
        //
        // However, this does skip flags with values in the same argument
        // (--foo=bar) so preserve it for that. Note that Go version always
        // skips the next argument if the current argument is a flag, even if
        // the current argument has has a =, but that doesn't look correct so we
        // don't do that.
        if arg.contains('=') {
            continue;
        }

        if arg.starts_with('-') {
            // Skip next argument which is likely the flag's value. This is
            // probably wrong for many cases since many flags do not take value,
            // but this is what the Go implementation does so we leave it as is
            // for now.
            //
            // Typically, in Ruby, the script name comes first so it probably
            // doesn't matter much in practice.
            args.next();
            continue;
        }

        let name = Path::new(arg)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or(arg);
        let name = name
            .split_once(':')
            .map(|(prefix, _)| prefix)
            .unwrap_or(name);
        if name.chars().next().is_some_and(|c| c.is_alphabetic()) {
            return Some(ServiceNameMetadata::new(
                name,
                ServiceNameSource::CommandLine,
            ));
        }
    }

    None
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::*;
    use crate::cmdline;

    #[test]
    fn test_ruby_td_agent() {
        // Test case from Go: ruby - td-agent
        let cmdline = cmdline![
            "ruby",
            "/usr/sbin/td-agent",
            "--log",
            "/var/log/td-agent/td-agent.log",
            "--daemon",
            "/var/run/td-agent/td-agent.pid"
        ];
        let result = extract_name(&cmdline);
        assert!(result.is_some());
        let result = result.unwrap();
        assert_eq!(result.name, "td-agent");
        assert_eq!(result.source, ServiceNameSource::CommandLine);
    }

    #[test]
    fn test_ruby_with_flags() {
        // Test that flags and their values are skipped when they come after the script
        // In typical Ruby usage, the script comes first, then flags
        let cmdline = cmdline!["ruby", "/usr/bin/sidekiq", "--config", "config.yml", "-w"];
        let result = extract_name(&cmdline);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name, "sidekiq");
    }

    #[test]
    fn test_ruby_with_flag_values() {
        let cmdline = cmdline!["ruby", "--config=config.yml", "/app/script/worker.rb"];
        let result = extract_name(&cmdline);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name, "worker.rb");
    }

    #[test]
    fn test_ruby_colon_trimming() {
        // Test that colons are trimmed
        let cmdline = cmdline!["ruby", "sidekiq:worker"];
        let result = extract_name(&cmdline);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name, "sidekiq");
    }

    #[test]
    fn test_ruby_nothing_before_colon() {
        let cmdline = cmdline!["ruby", ":worker"];
        let result = extract_name(&cmdline);
        assert!(result.is_none());
    }

    #[test]
    fn test_ruby_non_alphabetic_name_skipped() {
        // Unclear if some like this actually happens, but the Go version has
        // the is_alphabetic() check for some reason so exercise it in a test.
        let cmdline = cmdline!["ruby", "+foo", "bar"];
        let result = extract_name(&cmdline);
        assert!(result.is_some());
        assert_eq!(result.unwrap().name, "bar");
    }

    #[test]
    fn test_ruby_no_valid_arg() {
        // Test when no valid argument is found
        let cmdline = cmdline!["ruby", "-e", "puts 'hello'"];
        let result = extract_name(&cmdline);
        assert!(result.is_none());
    }

    #[test]
    fn test_ruby_only_executable() {
        // Test when only executable is present
        let cmdline = cmdline!["ruby"];
        let result = extract_name(&cmdline);
        assert!(result.is_none());
    }
}
