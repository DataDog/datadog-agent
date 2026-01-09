// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use super::{Deployment, DeploymentType, Error, abs};
use crate::service_name::context::DetectionContext;
use serde::Deserialize;
use std::collections::HashSet;
use std::ffi::OsStr;
use std::io::BufReader;
use std::path::{Path, PathBuf};

const SERVER_XML_PATH: &str = "conf/server.xml";
const ROOT_WEB_APP: &str = "ROOT";

/// Tomcat server.xml structure
#[derive(Debug, Deserialize)]
struct ServerXML {
    #[serde(rename = "Service", default)]
    services: Vec<Service>,
}

#[derive(Debug, Deserialize)]
struct Service {
    #[serde(rename = "Engine", default)]
    engine: Option<Engine>,
}

#[derive(Debug, Deserialize)]
struct Engine {
    #[serde(rename = "Host", default)]
    hosts: Vec<Host>,
}

#[derive(Debug, Deserialize)]
struct Host {
    #[serde(rename = "@appBase")]
    app_base: String,
    #[serde(rename = "Context", default)]
    contexts: Vec<TomcatContext>,
}

#[derive(Debug, Deserialize)]
struct TomcatContext {
    #[serde(rename = "@docBase", default)]
    doc_base: String,
    #[serde(rename = "@path", default)]
    path: String,
}

fn parse_server_xml(ctx: &DetectionContext, domain_home: &Path) -> Result<ServerXML, Error> {
    let xml_path = domain_home.join(SERVER_XML_PATH);
    let file = ctx.fs.open(&xml_path)?;
    let reader = BufReader::new(file.verify(None)?);
    quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse server.xml: {}", e)))
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

    for service in server_xml.services {
        let Some(engine) = service.engine else {
            continue;
        };

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
}
