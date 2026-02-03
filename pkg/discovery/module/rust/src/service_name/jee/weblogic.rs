// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use super::{Deployment, DeploymentFs, Error, abs, extract_java_property_from_args};
use crate::procfs::Cmdline;
use crate::service_name::context::DetectionContext;
use serde::Deserialize;
use std::io::BufReader;
use std::path::Path;

const SERVER_NAME_SYS_PROP: &str = "-Dweblogic.Name=";
const SERVER_CONFIG_FILE: &str = "config.xml";
const SERVER_CONFIG_DIR: &str = "config";
const XML_FILE: &str = "META-INF/weblogic.xml";

/// Weblogic deployment info from config.xml
#[derive(Debug, Deserialize)]
struct DeploymentInfo {
    #[serde(rename = "app-deployment", default)]
    app_deployment: Vec<AppDeployment>,
}

#[derive(Debug, Deserialize)]
struct AppDeployment {
    #[serde(rename = "target")]
    target: String,
    #[serde(rename = "source-path")]
    source_path: String,
    #[serde(rename = "staging-mode")]
    staging_mode: String,
}

/// Weblogic-specific context root from weblogic.xml
#[derive(Debug, Deserialize)]
struct ContextRoot {
    #[serde(rename = "context-root", default)]
    context_root: String,
}

pub fn find_deployed_apps(
    cmdline: &Cmdline,
    ctx: &DetectionContext,
    domain_home: &Path,
) -> Result<Vec<Deployment>, Error> {
    let server_name = extract_java_property_from_args(cmdline, SERVER_NAME_SYS_PROP)
        .ok_or_else(|| Error::MissingConfig("weblogic.Name property not found".to_string()))?;

    let config_path = domain_home.join(SERVER_CONFIG_DIR).join(SERVER_CONFIG_FILE);

    let file = ctx.fs.open(&config_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let deploy_infos: DeploymentInfo = quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse config.xml: {}", e)))?;

    let deployments: Vec<_> = deploy_infos
        .app_deployment
        .into_iter()
        .filter(|di| di.staging_mode == "stage" && di.target == server_name)
        .map(|di| {
            let path_obj = Path::new(&di.source_path);
            let name = path_obj
                .file_name()
                .and_then(|n| n.to_str())
                .unwrap_or(&di.source_path)
                .to_string();

            let path = abs(&di.source_path, domain_home);

            Deployment {
                name,
                path,
                kind: None,
                context_root: None,
            }
        })
        .collect();

    Ok(deployments)
}

pub fn custom_extract_war_context_root(deployment_fs: &mut DeploymentFs) -> Option<String> {
    let buf = deployment_fs.read_file_to_vec(XML_FILE).ok()?;
    let wls_xml: ContextRoot = quick_xml::de::from_reader(buf.as_slice()).ok()?;

    if wls_xml.context_root.is_empty() {
        None
    } else {
        Some(wls_xml.context_root)
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::indexing_slicing)]
mod tests {
    use super::*;
    use crate::fs::SubDirFs;
    use crate::service_name::context::DetectionContext;
    use crate::test_utils::TestDataFs;
    use std::collections::HashMap;
    use std::fs;
    use std::io::Write;
    use std::path::PathBuf;
    use tempfile::TempDir;
    use zip::ZipWriter;
    use zip::write::SimpleFileOptions;

    const TEST_APP_ROOT: &str = "../sub";
    const TEST_APP_ROOT_ABSOLUTE: &str = "/sub";

    // TestWeblogicFindDeployedApps tests the ability to extract deployed applications from a weblogic config.xml
    // The file contains staged and non-staged deployments for different servers.
    // It is expected that only the staged deployment of `AdminServer` are returned.
    #[test]
    fn test_weblogic_find_deployed_apps() {
        enum FsSetup {
            RealTestData,
            Empty,
            MalformedXml,
        }

        use super::super::tests::ErrorChecker;

        struct TestCase {
            name: &'static str,
            server_name: Option<&'static str>,
            domain_home: &'static str,
            fs_setup: FsSetup,
            expected: Option<Vec<(&'static str, String)>>, // (name, path)
            expected_error: Option<ErrorChecker>,
        }

        let tests = vec![
            TestCase {
                name: "multiple deployments for multiple server - extract for AdminServer",
                server_name: Some("AdminServer"),
                domain_home: TEST_APP_ROOT_ABSOLUTE,
                fs_setup: FsSetup::RealTestData,
                expected: Some(vec![
                    ("test.war", format!("{}/test.war", TEST_APP_ROOT_ABSOLUTE)),
                    (
                        "sample4.war",
                        "/u01/oracle/user_projects/tmp/sample4.war".to_string(),
                    ),
                    ("test.ear", format!("{}/test.ear", TEST_APP_ROOT_ABSOLUTE)),
                ]),
                expected_error: None,
            },
            TestCase {
                name: "server name is missing",
                server_name: None,
                domain_home: TEST_APP_ROOT,
                fs_setup: FsSetup::RealTestData,
                expected: None,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "missing config.xml",
                server_name: Some("AdminServer"),
                domain_home: TEST_APP_ROOT,
                fs_setup: FsSetup::Empty,
                expected: None,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::Io(_)),
                        "Expected JeeError::Io but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "malformed config.xml",
                server_name: Some("AdminServer"),
                domain_home: "weblogic",
                fs_setup: FsSetup::MalformedXml,
                expected: None,
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
            let test_data_fs;
            let subdirfs;
            let fs_root = match tt.fs_setup {
                FsSetup::RealTestData => {
                    test_data_fs = TestDataFs::new("jee/weblogic");
                    test_data_fs.as_ref()
                }
                FsSetup::Empty => {
                    subdirfs = SubDirFs::new(tmp_dir.path()).unwrap();
                    &subdirfs
                }
                FsSetup::MalformedXml => {
                    let config_dir = tmp_dir.path().join("weblogic").join("config");
                    fs::create_dir_all(&config_dir).unwrap();
                    fs::write(config_dir.join("config.xml"), b"evil").unwrap();
                    subdirfs = SubDirFs::new(tmp_dir.path()).unwrap();
                    &subdirfs
                }
            };

            let args: Vec<String> = tt
                .server_name
                .map(|name| vec![format!("-Dweblogic.Name={}", name)])
                .unwrap_or_default();

            let envs = HashMap::new();
            let ctx = DetectionContext::new(1, envs, fs_root);
            let domain_home = PathBuf::from(tt.domain_home);

            let args_strs: Vec<&str> = args.iter().map(|s| s.as_str()).collect();
            let cmdline = crate::procfs::Cmdline::from(&args_strs[..]);
            let result = find_deployed_apps(&cmdline, &ctx, &domain_home);

            match tt.expected {
                Some(expected) => {
                    assert!(
                        result.is_ok(),
                        "{}: find_deployed_apps returned Err: {:?}",
                        tt.name,
                        result
                    );
                    let deployments = result.unwrap();
                    assert_eq!(
                        deployments.len(),
                        expected.len(),
                        "{}: expected {} deployments, got {}",
                        tt.name,
                        expected.len(),
                        deployments.len()
                    );

                    for (exp_name, exp_path) in &expected {
                        let found = deployments
                            .iter()
                            .find(|d| d.name == *exp_name && d.path == *exp_path);
                        assert!(
                            found.is_some(),
                            "{}: expected deployment {} at {} not found",
                            tt.name,
                            exp_name,
                            exp_path
                        );
                    }
                }
                None => {
                    assert!(
                        result.is_err() || result.as_ref().unwrap().is_empty(),
                        "{}: expected error or empty result, got {:?}",
                        tt.name,
                        result
                    );

                    // Verify error variant if callback provided
                    if result.is_err()
                        && let Some(check_error) = tt.expected_error
                    {
                        let err = result.unwrap_err();
                        check_error(&err);
                    }
                }
            }
        }
    }

    #[test]
    fn test_weblogic_extract_war_context_root() {
        struct TestCase {
            name: &'static str,
            xml_content: Option<&'static str>,
            expected: Option<&'static str>,
        }

        let tests = vec![
            TestCase {
                name: "war with weblogic.xml and context-root",
                xml_content: Some(
                    r#"
<weblogic-web-app xmlns="http://xmlns.oracle.com/weblogic/weblogic-web-app" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.oracle.com/weblogic/weblogic-web-app
http://xmlns.oracle.com/weblogic/weblogic-web-app/1.4/weblogic-web-app.xsd">
<context-root>my-context</context-root>
</weblogic-web-app>"#,
                ),
                expected: Some("my-context"),
            },
            TestCase {
                name: "weblogic.xml without context-root",
                xml_content: Some(
                    r#"
<weblogic-web-app xmlns="http://xmlns.oracle.com/weblogic/weblogic-web-app" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.oracle.com/weblogic/weblogic-web-app
http://xmlns.oracle.com/weblogic/weblogic-web-app/1.4/weblogic-web-app.xsd"/>"#,
                ),
                expected: None,
            },
            TestCase {
                name: "broken weblogic.xml",
                xml_content: Some(
                    r#"
<weblogic-web-app xmlns="http://xmlns.oracle.com/weblogic/weblogic-web-app" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
xsi:schemaLocation="http://xmlns.oracle.com/weblogic/weblogic-web-app
http://xmlns.oracle.com/weblogic/weblogic-web-app/1.4/weblogic-web-app.xsd">invalid!unfinished!"#,
                ),
                expected: None,
            },
            TestCase {
                name: "no weblogic.xml in the war",
                xml_content: None,
                expected: None,
            },
        ];

        for tt in tests {
            // Create an in-memory zip to emulate a WAR
            let mut zip_buf = Vec::new();
            {
                let mut zip_writer = ZipWriter::new(std::io::Cursor::new(&mut zip_buf));
                if let Some(xml_content) = tt.xml_content {
                    zip_writer
                        .start_file(XML_FILE, SimpleFileOptions::default())
                        .unwrap();
                    zip_writer.write_all(xml_content.as_bytes()).unwrap();
                }
                zip_writer.finish().unwrap();
            }

            // Now create a zip archive to pass to the tested function
            let tmp_dir = TempDir::new().unwrap();
            let war_path = tmp_dir.path().join("test.war");
            fs::write(&war_path, &zip_buf).unwrap();

            let fs_root = SubDirFs::new(tmp_dir.path()).unwrap();
            let file = fs_root.open(Path::new("test.war")).unwrap();
            let zip = file.verify_zip().unwrap();
            let mut deployment_fs = DeploymentFs::ZipArchive(zip);

            let result = custom_extract_war_context_root(&mut deployment_fs);

            match tt.expected {
                Some(expected) => {
                    assert_eq!(
                        result,
                        Some(expected.to_string()),
                        "{}: expected Some({:?}), got {:?}",
                        tt.name,
                        expected,
                        result
                    );
                }
                None => {
                    assert_eq!(result, None, "{}: expected None, got {:?}", tt.name, result);
                }
            }
        }
    }

    // TestWeblogicExtractExplodedWarContextRoot tests the ability to extract context root from weblogic.xml
    // when the deployment is exploded (aka is a directory and not a war archive)
    #[test]
    fn test_weblogic_extract_exploded_war_context_root() {
        let fs = TestDataFs::new("jee/weblogic");
        let subdirfs: &SubDirFs = fs.as_ref();
        let mut deployment_fs =
            DeploymentFs::Directory(subdirfs.sub(Path::new("test.war")).unwrap());

        let result = custom_extract_war_context_root(&mut deployment_fs);
        assert_eq!(result, Some("my_context".to_string()));
    }
}
