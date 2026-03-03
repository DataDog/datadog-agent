// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

mod context;
mod dotnet;
mod gunicorn;
mod java;
mod jee;
mod nodejs;
mod php;
mod python;
mod rails;
mod ruby;
mod spring;
mod uvicorn;

use std::path::Path;

use serde::Serialize;

use crate::{Language, procfs::Cmdline};
pub use context::DetectionContext;

pub mod erlang;

#[derive(Debug, PartialEq)]
pub struct ServiceNameMetadata {
    pub name: String,
    pub source: ServiceNameSource,
    pub additional_names: Vec<String>,
}

impl ServiceNameMetadata {
    pub fn new(name: impl Into<String>, source: ServiceNameSource) -> Self {
        Self {
            name: name.into(),
            source,
            additional_names: vec![],
        }
    }

    pub fn with_additional_names(mut self, mut additional_names: Vec<String>) -> Self {
        // Names are discovered in unpredictable order. We need to keep them sorted.
        additional_names.sort();
        self.additional_names = additional_names;
        self
    }
}

#[derive(Clone, Debug, Serialize, PartialEq)]
#[serde(rename_all = "kebab-case")]
pub enum ServiceNameSource {
    CommandLine,
    Erlang,
    Laravel,
    Python,
    Nodejs,
    Gunicorn,
    Rails,
    Spring,
    Jboss,
    Tomcat,
    Weblogic,
    Websphere,
}

impl ServiceNameSource {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::CommandLine => "command-line",
            Self::Erlang => "erlang",
            Self::Laravel => "laravel",
            Self::Python => "python",
            Self::Nodejs => "nodejs",
            Self::Gunicorn => "gunicorn",
            Self::Rails => "rails",
            Self::Spring => "spring",
            Self::Jboss => "jboss",
            Self::Tomcat => "tomcat",
            Self::Weblogic => "weblogic",
            Self::Websphere => "websphere",
        }
    }
}

pub fn get(
    language: &Language,
    cmdline: &Cmdline,
    ctx: &mut DetectionContext,
) -> Option<ServiceNameMetadata> {
    if cmdline.is_empty() {
        return None;
    }

    let mut exe = cmdline.args().next()?;

    // Trim any quotes from the executable (can happen on Windows)
    exe = exe.trim_matches('"');
    exe = Path::new(exe)
        .file_name()
        .unwrap_or_default()
        .to_str()
        .unwrap_or("");
    exe = trim_at_sep_end(exe, ':');
    exe = trim_symbols_from_exe(exe);
    exe = normalize_exe_name(exe);

    match exe {
        "gunicorn" => Some(gunicorn::extract_name(cmdline, &ctx.envs)),
        "puma" => rails::extract_name(cmdline, ctx),
        "beam.smp" | "beam" => erlang::extract_name(cmdline),
        "php" => php::extract_name(cmdline, ctx),
        &_ => match language {
            Language::Python => python::extract_name(cmdline, ctx),
            Language::Ruby => ruby::extract_name(cmdline),
            Language::Java => java::extract_name(cmdline, ctx),
            Language::NodeJS => nodejs::extract_name(cmdline, ctx),
            Language::DotNet => dotnet::extract_name(cmdline),
            Language::PHP => php::extract_name(cmdline, ctx),
            _ => {
                if let Some(idx) = exe.find('.') {
                    exe = exe.get(..idx)?;
                }

                Some(ServiceNameMetadata::new(
                    exe,
                    ServiceNameSource::CommandLine,
                ))
            }
        },
    }
}

fn trim_at_sep_end(s: &str, sep: char) -> &str {
    if let Some(idx) = s.find(sep) {
        s.get(..idx).unwrap_or(s)
    } else {
        s
    }
}

// Returns the exe name without the special chars that it has at the beginning,
// and (maybe) at the end.
fn trim_symbols_from_exe(exe: &str) -> &str {
    match exe.chars().next() {
        None => exe,
        Some(c) if c.is_alphabetic() => exe,
        Some(c) => exe
            .get(c.len_utf8()..)
            .and_then(|exe| {
                exe.chars()
                    .next_back()
                    .filter(|c| !c.is_alphabetic())
                    .and_then(|c| {
                        let end = exe.len().saturating_sub(c.len_utf8());
                        exe.get(..end)
                    })
            })
            .unwrap_or(exe),
    }
}

fn normalize_exe_name(exe: &str) -> &str {
    if let Some(suffix) = exe.strip_prefix("php")
        && is_php_valid_version(suffix)
    {
        return "php";
    }

    exe
}

/// Checks that the provided string is a valid PHP version number (i.e numbers
/// separate with dots)
/// TODO: This will be tested when adding the PHP support
fn is_php_valid_version(version: &str) -> bool {
    if version.is_empty() {
        return true;
    }

    for v in version.split('.') {
        for c in v.chars() {
            if !c.is_ascii_digit() {
                return false;
            }
        }
    }

    true
}

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod tests {
    use super::get as get_name;
    use super::{DetectionContext, ServiceNameMetadata, ServiceNameSource};
    use crate::fs::SubDirFs;
    use crate::procfs::Cmdline;
    use crate::test_utils::TestDataFs;
    use crate::{Language, cmdline};
    use std::collections::HashMap;

    fn test_ctx() -> (HashMap<String, String>, SubDirFs) {
        let tempdir = std::env::temp_dir();
        let fs = SubDirFs::new(&tempdir).unwrap();
        let envs = HashMap::new();
        (envs, fs)
    }

    #[test]
    fn empty_cmdline() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(get_name(&Language::Unknown, &cmdline![], &mut ctx), None);
    }

    #[test]
    fn single_arg_exe() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(&Language::Unknown, &cmdline!["./my-server.sh"], &mut ctx),
            Some(ServiceNameMetadata::new(
                "my-server",
                ServiceNameSource::CommandLine,
            ))
        );

        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(&Language::Unknown, &cmdline!["./-my-server.sh-"], &mut ctx),
            Some(ServiceNameMetadata::new(
                "my-server",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_integration_erlang_detect() {
        // Integration test: verifies the complete flow (dispatch → Erlang detector → result)
        // Tests that beam.smp processes are correctly routed to the Erlang detector
        let cmdline = cmdline!["beam.smp", "-progname", "erl", "-home", "/var/lib/rabbitmq"];
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let result = get_name(&Language::Unknown, &cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "rabbitmq",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_integration_erlang_detect_with_beam() {
        // Integration test with "beam" executable (not "beam.smp")
        let cmdline = cmdline!["beam", "-progname", "couchdb", "-home", "/opt/couchdb"];
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let result = get_name(&Language::Unknown, &cmdline, &mut ctx);
        assert_eq!(
            result,
            Some(ServiceNameMetadata::new(
                "couchdb",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn test_integration_erlang_no_name() {
        // Integration test: Erlang process without valid name returns None
        let cmdline = cmdline!["beam.smp", "-smp", "auto", "-noinput"];
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        let result = get_name(&Language::Unknown, &cmdline, &mut ctx);
        assert_eq!(result, None);
    }

    #[test]
    fn node_js_integration() {
        // Integration test to ensure get_name() properly calls into nodejs
        // module. Further tests are in the module itself.
        let fs = TestDataFs::new("nodejs");
        let envs = HashMap::new();
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        assert_eq!(
            get_name(
                &Language::NodeJS,
                &cmdline!["/usr/bin/node", "./testdata/index.js"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "my-awesome-package",
                ServiceNameSource::Nodejs,
            ))
        );
    }

    #[test]
    fn rails_integration() {
        // Integration test to ensure get_name() properly calls into rails module.
        // Further tests are in the module itself.
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Ruby,
                &cmdline!["puma", "5.6.5", "(cluster)", "[api_gateway_service]"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "api_gateway_service",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn dotnet_integration() {
        // Integration test to ensure get_name() properly calls into dotnet module.
        // Further tests are in the module itself.
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::DotNet,
                &cmdline!["/usr/bin/dotnet", "./myservice.dll"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "myservice",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn ruby_integration() {
        // Integration test for generic Ruby process (not puma/rails)
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Ruby,
                &cmdline![
                    "ruby",
                    "/usr/sbin/td-agent",
                    "--log",
                    "/var/log/td-agent/td-agent.log",
                    "--daemon",
                    "/var/run/td-agent/td-agent.pid"
                ],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "td-agent",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn python_integration() {
        // Integration test to ensure get_name() properly calls into python
        // module. Further tests are in the module itself.
        let fs = TestDataFs::new("python");
        let mut envs = HashMap::new();
        envs.insert("PWD".to_string(), "/".to_string());
        let mut ctx = DetectionContext::new(0, envs, fs.as_ref());
        assert_eq!(
            get_name(
                &Language::Python,
                &cmdline!["python", "modules/m1/first/nice/something.py"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "m1.first.nice.something",
                ServiceNameSource::Python,
            ))
        );
    }

    #[test]
    fn java_integration() {
        // Integration test to ensure get_name() properly calls into java
        // module. Further tests are in the module itself.
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Java,
                &cmdline![
                    "java",
                    "-Xmx4000m",
                    "-jar",
                    "/opt/sheepdog/bin/myservice.jar"
                ],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "myservice",
                ServiceNameSource::CommandLine,
            ))
        );
    }

    #[test]
    fn php_integration_artisan() {
        // Integration test to ensure get_name() properly calls into php module.
        // Further tests are in the module itself.
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::PHP,
                &cmdline!["php", "artisan", "serve"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "laravel",
                ServiceNameSource::Laravel
            ))
        );
    }

    #[test]
    fn php_integration_datadog_service() {
        // Integration test for PHP with datadog.service flag
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::PHP,
                &cmdline!["php", "-ddatadog.service=my-php-service", "server.php"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "my-php-service",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn gunicorn_integration_simple() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Python,
                &cmdline!["gunicorn", "--workers=2", "test:app"],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "test",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn gunicorn_integration_with_python() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Python,
                &cmdline![
                    "/usr/bin/python3",
                    "/usr/bin/gunicorn",
                    "--workers=2",
                    "foo:create_app()"
                ],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "foo",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn test_gunicorn_integration_packed() {
        // Simulate gunicorn's behavior of setting process title with trailing
        // nulls. This is what is in /proc/<pid>/cmdline for gunicorn processes
        // started with the -n flag (with setproctitle installed).
        let cmdline = Cmdline::new("gunicorn: master [myapp]\0\0\0\0\0\0\0\0".to_string());

        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);

        let service_name = get_name(&Language::Python, &cmdline, &mut ctx);

        assert_eq!(
            service_name,
            Some(ServiceNameMetadata::new(
                "myapp",
                ServiceNameSource::CommandLine
            ))
        );
    }

    #[test]
    fn uvicorn_integration() {
        let (envs, fs) = test_ctx();
        let mut ctx = DetectionContext::new(0, envs, &fs);
        assert_eq!(
            get_name(
                &Language::Python,
                &cmdline![
                    "/usr/local/bin/python",
                    "/usr/local/bin/uvicorn",
                    "myapp.asgi:application",
                    "--host=0.0.0.0",
                    "--port=8000"
                ],
                &mut ctx
            ),
            Some(ServiceNameMetadata::new(
                "myapp.asgi",
                ServiceNameSource::CommandLine
            ))
        );
    }
}
