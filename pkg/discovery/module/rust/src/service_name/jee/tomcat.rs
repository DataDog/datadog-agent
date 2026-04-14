// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use super::xml_parser::{self, Action, XmlHandler, XmlParser};
use super::{Deployment, DeploymentType, Error, abs};
use crate::service_name::context::DetectionContext;
use std::collections::HashSet;
use std::ffi::OsStr;
use std::io::BufReader;
use std::path::{Path, PathBuf};
use xml::attribute::OwnedAttribute;

const SERVER_XML_PATH: &str = "conf/server.xml";
const ROOT_WEB_APP: &str = "ROOT";

/// Parsed Tomcat server.xml: only the engines (with their hosts) matter.
#[derive(Debug)]
struct ServerXML {
    engines: Vec<Engine>,
}

#[derive(Debug)]
struct Engine {
    hosts: Vec<Host>,
}

/// A `<Host>` element from server.xml.
///
/// Missing `appBase` defaults to `""`, matching Go's `encoding/xml`
/// (the reference implementation).  The old quick-xml/serde parser
/// rejected the document if `@appBase` was absent.  An empty `appBase`
/// resolves to `domain_home` via [`abs`], which may cause
/// `scan_dir_for_deployments` to enumerate the Tomcat root — this
/// matches Go's behavior for the same malformed input.
#[derive(Debug)]
struct Host {
    app_base: String,
    contexts: Vec<TomcatContext>,
}

#[derive(Debug)]
struct TomcatContext {
    doc_base: String,
    path: String,
}

enum ServerXmlState {
    Top,
    InServer,
    InService,
    InEngine { hosts: Vec<Host> },
    InHost { hosts: Vec<Host>, current: Host },
}

struct ServerXmlHandler {
    result: ServerXML,
}

impl XmlHandler for ServerXmlHandler {
    type State = ServerXmlState;

    fn start_element(
        &mut self,
        state: Self::State,
        name: &str,
        attributes: &[OwnedAttribute],
    ) -> Action<Self::State> {
        match (state, name) {
            (ServerXmlState::Top, "Server") => Action::Descend(ServerXmlState::InServer),
            (ServerXmlState::InServer, "Service") => Action::Descend(ServerXmlState::InService),
            (ServerXmlState::InService, "Engine") => {
                Action::Descend(ServerXmlState::InEngine { hosts: Vec::new() })
            }
            (ServerXmlState::InEngine { hosts }, "Host") => {
                let app_base = xml_parser::get_attr(attributes, "appBase").unwrap_or_default();
                Action::Descend(ServerXmlState::InHost {
                    hosts,
                    current: Host {
                        app_base,
                        contexts: Vec::new(),
                    },
                })
            }
            (ServerXmlState::InHost { hosts, mut current }, "Context") => {
                let doc_base = xml_parser::get_attr(attributes, "docBase").unwrap_or_default();
                let path = xml_parser::get_attr(attributes, "path").unwrap_or_default();
                current.contexts.push(TomcatContext { doc_base, path });
                Action::Same(ServerXmlState::InHost { hosts, current })
            }
            (s, _) => Action::Same(s),
        }
    }

    fn end_element(&mut self, state: Self::State, name: &str) -> Action<Self::State> {
        match (state, name) {
            (ServerXmlState::InHost { mut hosts, current }, "Host") => {
                hosts.push(current);
                Action::Ascend(ServerXmlState::InEngine { hosts })
            }
            (ServerXmlState::InEngine { hosts }, "Engine") => {
                self.result.engines.push(Engine { hosts });
                Action::Ascend(ServerXmlState::InService)
            }
            (ServerXmlState::InService, "Service") => Action::Ascend(ServerXmlState::InServer),
            (ServerXmlState::InServer, "Server") => Action::Break,
            (s, _) => Action::Same(s),
        }
    }
}

fn parse_server_xml(ctx: &DetectionContext, domain_home: &Path) -> Result<ServerXML, Error> {
    let xml_path = domain_home.join(SERVER_XML_PATH);
    let file = ctx.fs.open(&xml_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let mut parser = XmlParser::new(reader);
    let mut handler = ServerXmlHandler {
        result: ServerXML {
            engines: Vec::new(),
        },
    };
    parser.run(&mut handler, ServerXmlState::Top)?;
    Ok(handler.result)
}

fn scan_dir_for_deployments(
    ctx: &DetectionContext,
    path: &PathBuf,
    uniques: &mut HashSet<(String, PathBuf)>,
) -> impl Iterator<Item = Deployment> {
    ctx.fs
        .read_dir(path)
        .ok()
        .into_iter()
        .flatten()
        .filter_map(Result::ok)
        .filter_map(move |entry| {
            let deployment = create_deployment_from_filepath(&entry.file_name(), path);
            let key = (deployment.name.clone(), path.clone());
            if uniques.insert(key) {
                Some(deployment)
            } else {
                None
            }
        })
}

pub fn find_deployed_apps(
    ctx: &DetectionContext,
    domain_home: &Path,
) -> Result<Vec<Deployment>, Error> {
    let server_xml = parse_server_xml(ctx, domain_home)?;

    let mut deployments = Vec::new();
    let mut uniques: HashSet<(String, PathBuf)> = HashSet::new();

    for engine in server_xml.engines {
        for host in engine.hosts {
            let app_base = abs(&host.app_base, domain_home);

            // Process explicit contexts
            for context in host.contexts {
                if context.doc_base.is_empty() || context.path.is_empty() {
                    continue;
                }

                let doc_base_path = abs(&context.doc_base, &app_base);
                let Some(file_name) = doc_base_path.file_name() else {
                    continue;
                };
                let deployment = create_deployment_from_filepath(file_name, &app_base);

                // The Go code is unclear about what is the key for
                // deduplication.  In the explicit contexts case, it
                // uses the contextRoot as the key when checking for
                // duplicates, but when inserting, it uses the
                // deployment name.  This looks like a bug, so we don't
                // duplicate that behavior, but instead use a key of
                // (name, path) to track unique deployments across all
                // hosts, which is what the test in the Go code appears
                // to expect.
                let key = (deployment.name.clone(), app_base.clone());
                if uniques.insert(key) {
                    deployments.push(Deployment {
                        name: deployment.name,
                        path: deployment.path,
                        kind: Some(DeploymentType::War),
                        context_root: Some(context.path),
                    });
                }
            }

            // Scan appBase directory for additional deployments
            deployments.extend(scan_dir_for_deployments(ctx, &app_base, &mut uniques));
        }
    }

    Ok(deployments)
}

pub fn default_context_root_from_file(file_name: &str) -> Option<String> {
    let keep = if let Some((before, _)) = file_name.split_once("##") {
        before
    } else {
        file_name
            .rsplit_once('.')
            .map(|(before, _)| before)
            .unwrap_or(file_name)
    };

    if keep == ROOT_WEB_APP {
        return None;
    }

    Some(keep.replace('#', "/"))
}

fn create_deployment_from_filepath(file_name: &OsStr, dir: &Path) -> Deployment {
    let file_name = file_name.to_string_lossy();
    let stripped = file_name
        .rsplit_once('.')
        .map(|(name, _)| name)
        .unwrap_or(&file_name);

    Deployment {
        path: dir.to_path_buf(),
        name: stripped.to_string(),
        kind: Some(DeploymentType::War),
        context_root: None,
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::indexing_slicing)]
mod tests {
    use super::*;
    use crate::fs::SubDirFs;
    use crate::service_name::context::DetectionContext;
    use std::collections::HashMap;
    use std::fs;
    use tempfile::TempDir;

    #[test]
    fn test_default_context_root_from_file() {
        struct Test {
            filename: &'static str,
            expected: Option<&'static str>,
        }

        let tests = vec![
            Test {
                filename: "foo.war",
                expected: Some("foo"),
            },
            Test {
                filename: "foo",
                expected: Some("foo"),
            },
            Test {
                filename: "foo#bar.war",
                expected: Some("foo/bar"),
            },
            Test {
                filename: "ROOT.war",
                expected: None,
            },
            Test {
                filename: "foo##10.war",
                expected: Some("foo"),
            },
            Test {
                filename: "foo#bar##15",
                expected: Some("foo/bar"),
            },
            Test {
                filename: "ROOT##666",
                expected: None,
            },
        ];

        for tt in tests {
            let value = default_context_root_from_file(tt.filename);
            let expected = tt.expected.map(|s| s.to_string());
            assert_eq!(value, expected, "Should parse {}", tt.filename);
        }
    }

    #[test]
    fn test_scan_dir_for_deployments() {
        struct Test {
            path: &'static str,
            expected: Vec<Deployment>,
        }

        let tests = vec![
            Test {
                // "dir not exist",
                path: "nowhere",
                expected: vec![],
            },
            Test {
                // "should dedupe deployments",
                path: "webapps",
                expected: vec![
                    Deployment {
                        name: "app1".to_string(),
                        path: PathBuf::from("webapps"),
                        kind: Some(DeploymentType::War),
                        context_root: None,
                    },
                    Deployment {
                        name: "app2".to_string(),
                        path: PathBuf::from("webapps"),
                        kind: Some(DeploymentType::War),
                        context_root: None,
                    },
                ],
            },
        ];

        for tt in tests {
            let tmp_dir = TempDir::new().unwrap();
            let base = tmp_dir.path();

            // Create test directory structure
            let webapps = base.join("webapps");
            fs::create_dir_all(&webapps).unwrap();

            // Create deployments: app1.war, app1 dir, app2 dir, app3 dir
            fs::write(webapps.join("app1.war"), b"").unwrap();
            fs::create_dir(webapps.join("app1")).unwrap();
            fs::create_dir(webapps.join("app2")).unwrap();
            fs::create_dir(webapps.join("app3")).unwrap();

            let fs_root = SubDirFs::new(base).unwrap();
            let envs = HashMap::new();
            let ctx = DetectionContext::new(1, envs, &fs_root);

            let excluded = ["app3"];
            let mut uniques: HashSet<(String, PathBuf)> = excluded
                .iter()
                .map(|s| (s.to_string(), PathBuf::from(tt.path)))
                .collect();
            let mut deployments: Vec<_> =
                scan_dir_for_deployments(&ctx, &PathBuf::from(tt.path), &mut uniques).collect();

            deployments.sort_by(|d1, d2| d1.name.cmp(&d2.name));
            assert_eq!(deployments, tt.expected);
        }
    }

    #[test]
    fn test_find_deployed_apps() {
        use super::super::tests::ErrorChecker;

        struct Test {
            name: &'static str,
            setup: Box<dyn Fn(&TempDir)>,
            expected: Vec<Deployment>,
            should_succeed: bool,
            expected_error: Option<ErrorChecker>,
        }

        let tests = vec![
            Test {
                name: "tomcat - two virtual hosts",
                setup: Box::new(|tmp_dir: &TempDir| {
                    let base = tmp_dir.path();

                    // Create directory structure
                    let webapps1 = base.join("webapps1");
                    let webapps2 = base.join("webapps2");
                    fs::create_dir_all(&webapps1).unwrap();
                    fs::create_dir_all(&webapps2).unwrap();

                    // Create deployment files - app1.war and app1 dir should dedupe to just app1
                    fs::write(webapps1.join("app1.war"), b"").unwrap();
                    fs::create_dir(webapps1.join("app1")).unwrap();
                    fs::create_dir(webapps1.join("app2")).unwrap();
                    fs::create_dir(webapps2.join("app2")).unwrap();

                    // Create server.xml
                    let conf_dir = base.join("conf");
                    fs::create_dir(&conf_dir).unwrap();
                    let server_xml = r#"<Server port="8005" shutdown="SHUTDOWN">
  <Service name="Catalina">
    <Engine name="Catalina" defaultHost="host1">
		<Host name="host1"  appBase="webapps1"
				unpackWARs="true" autoDeploy="false">
			<Context docBase="app1" path="/context_1"/>
		</Host>
		<Host name="host2"  appBase="webapps2"
				unpackWARs="true" autoDeploy="false">
			<Context docBase="app2" path="/context_2"/>
		</Host>
	</Engine>
  </Service>
</Server>"#;
                    fs::write(conf_dir.join("server.xml"), server_xml).unwrap();
                }),
                expected: vec![
                    Deployment {
                        name: "app1".to_string(),
                        path: PathBuf::from("webapps1"),
                        kind: Some(DeploymentType::War),
                        context_root: Some("/context_1".to_string()),
                    },
                    Deployment {
                        name: "app2".to_string(),
                        path: PathBuf::from("webapps1"),
                        kind: Some(DeploymentType::War),
                        context_root: None,
                    },
                    Deployment {
                        name: "app2".to_string(),
                        path: PathBuf::from("webapps2"),
                        kind: Some(DeploymentType::War),
                        context_root: Some("/context_2".to_string()),
                    },
                ],
                should_succeed: true,
                expected_error: None,
            },
            Test {
                name: "missing configuration",
                setup: Box::new(|_tmp_dir: &TempDir| {
                    // No files created
                }),
                expected: vec![],
                should_succeed: false,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::Io(_)),
                        "Expected JeeError::Io but got {:?}",
                        err
                    );
                })),
            },
            Test {
                name: "malformed server configuration",
                setup: Box::new(|tmp_dir: &TempDir| {
                    let base = tmp_dir.path();
                    let conf_dir = base.join("conf");
                    fs::create_dir(&conf_dir).unwrap();
                    fs::write(conf_dir.join("server.xml"), b"bad").unwrap();
                }),
                expected: vec![],
                should_succeed: false,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::XmlParse(_)),
                        "Expected JeeError::XmlParse but got {:?}",
                        err
                    );
                })),
            },
        ];

        for tt in tests {
            let tmp_dir = TempDir::new().unwrap();
            (tt.setup)(&tmp_dir);

            let fs_root = SubDirFs::new(tmp_dir.path()).unwrap();
            let envs = HashMap::new();
            let ctx = DetectionContext::new(1, envs, &fs_root);

            let result = find_deployed_apps(&ctx, Path::new("."));

            if tt.should_succeed {
                assert!(
                    result.is_ok(),
                    "Test '{}' failed: expected success but got error: {:?}",
                    tt.name,
                    result.as_ref().err()
                );
                let deployments = result.unwrap();
                assert_eq!(
                    deployments.len(),
                    tt.expected.len(),
                    "Test '{}' failed: expected {} deployments, got {}. Actual: {:?}",
                    tt.name,
                    tt.expected.len(),
                    deployments.len(),
                    deployments
                );
                assert_eq!(
                    deployments, tt.expected,
                    "Test '{}' failed: deployments don't match",
                    tt.name
                );
            } else {
                assert!(
                    result.is_err(),
                    "Test '{}' failed: expected error but got success",
                    tt.name
                );

                if let Some(check_error) = tt.expected_error {
                    let err = result.unwrap_err();
                    check_error(&err);
                }
            }
        }
    }

    /// Worst-case memory test: many hosts × many contexts with large
    /// attributes, sized to fit within the 1 MiB file size cap.
    /// Measured peak heap with massif: ~2.1 MiB.
    #[test]
    fn test_worst_case_memory() {
        let n_hosts = 10;
        let n_contexts = 235;
        let pad: String = "x".repeat(200);

        let tmp_dir = TempDir::new().unwrap();
        let base = tmp_dir.path();

        let mut xml = String::from("<Server><Service name=\"s\"><Engine name=\"e\">");
        for h in 0..n_hosts {
            xml.push_str(&format!("<Host name=\"h{h}\" appBase=\"webapps{h}\">"));
            for c in 0..n_contexts {
                xml.push_str(&format!(
                    "<Context docBase=\"app{c}{pad}\" path=\"/ctx{c}{pad}\"/>"
                ));
            }
            xml.push_str("</Host>");
        }
        xml.push_str("</Engine></Service></Server>");
        assert!(xml.len() <= 1024 * 1024, "XML exceeds 1 MiB cap");

        let conf_dir = base.join("conf");
        fs::create_dir(&conf_dir).unwrap();
        fs::write(conf_dir.join("server.xml"), &xml).unwrap();

        let fs_root = SubDirFs::new(base).unwrap();
        let envs = HashMap::new();
        let ctx = DetectionContext::new(1, envs, &fs_root);

        let result = parse_server_xml(&ctx, Path::new(".")).unwrap();
        assert_eq!(result.engines.len(), 1);
        assert_eq!(result.engines[0].hosts.len(), n_hosts);
        for host in &result.engines[0].hosts {
            assert_eq!(host.contexts.len(), n_contexts);
        }
    }
}
