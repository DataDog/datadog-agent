// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use super::{Deployment, DeploymentFs, Error, extract_java_property_from_args};
use crate::fs::SubDirFs;
use crate::procfs::Cmdline;
use crate::service_name::context::DetectionContext;
use serde::Deserialize;
use std::borrow::Cow;
use std::collections::HashMap;
use std::io::BufReader;
use std::path::{Path, PathBuf};

const SERVER_NAME: &str = "-D[Server:";
const HOME_DIR_SYS_PROP: &str = "-Djboss.home.dir=";
const CONFIG_DIR: &str = "configuration";
const HOST_XML_FILE: &str = "host.xml";
const DEFAULT_STANDALONE_XML_FILE: &str = "standalone.xml";
const DEFAULT_DOMAIN_XML_FILE: &str = "domain.xml";
const DOMAIN_BASE: &str = "domain";
const STANDALONE_BASE: &str = "standalone";
const DATA_DIR: &str = "data";
const CONTENT_DIR: &str = "content";
const SERVER_CONFIG_SHORT: &str = "-c";
const SERVER_CONFIG: &str = "--server-config";
const DOMAIN_CONFIG: &str = "--domain-config";
const WEB_XML_FILE_META_INF: &str = "META-INF/jboss-web.xml";
const WEB_XML_FILE_WEB_INF: &str = "WEB-INF/jboss-web.xml";

/// Deployment content hash
#[derive(Debug, Clone, Deserialize)]
struct DeployedContent {
    #[serde(rename = "@sha1")]
    hash: String,
}

/// Server deployment information
#[derive(Debug, Clone, Deserialize)]
struct ServerDeployment {
    #[serde(rename = "@name")]
    name: String,
    #[serde(rename = "@runtime-name")]
    runtime_name: String,
    #[serde(rename = "@enabled", default)]
    enabled: Option<String>,
    #[serde(default)]
    content: Option<DeployedContent>,
}

/// Standalone XML structure
#[derive(Debug, Deserialize)]
struct StandaloneXML {
    #[serde(rename = "deployments", default)]
    deployments: Deployments,
}

#[derive(Debug, Deserialize, Default)]
struct Deployments {
    #[serde(rename = "deployment", default)]
    deployment: Vec<ServerDeployment>,
}

/// Domain XML structure
#[derive(Debug, Deserialize)]
struct DomainXML {
    #[serde(rename = "deployments", default)]
    deployments: Deployments,
    #[serde(rename = "server-groups", default)]
    server_groups: ServerGroups,
}

#[derive(Debug, Deserialize, Default)]
struct ServerGroups {
    #[serde(rename = "server-group", default)]
    server_group: Vec<ServerGroup>,
}

/// Server group information
#[derive(Debug, Deserialize)]
struct ServerGroup {
    #[serde(rename = "@name")]
    name: String,
    #[serde(rename = "deployments", default)]
    deployments: Deployments,
}

/// Host XML structure
#[derive(Debug, Deserialize)]
struct HostXML {
    #[serde(rename = "servers", default)]
    servers: Servers,
}

#[derive(Debug, Deserialize, Default)]
struct Servers {
    #[serde(rename = "server", default)]
    server: Vec<HostServer>,
}

/// Host server information (mapping server name to server group)
#[derive(Debug, Deserialize)]
struct HostServer {
    #[serde(rename = "@name")]
    name: String,
    #[serde(rename = "@group")]
    group: String,
}

/// JBoss-web.xml structure
#[derive(Debug, Deserialize)]
struct WebXML {
    #[serde(rename = "context-root", default)]
    context_root: String,
}

fn extract_server_name(cmdline: &Cmdline) -> (Option<String>, bool) {
    let mut domain_mode = false;
    if let Some(value) = extract_java_property_from_args(cmdline, SERVER_NAME) {
        domain_mode = true;

        // The property is in the form -D[Server:servername]. Remove the closing bracket
        if let Some(name) = value.strip_suffix(']')
            && !name.is_empty()
        {
            return (Some(name.to_string()), domain_mode);
        }
    }
    (None, domain_mode)
}

fn extract_config_filename(cmdline: &Cmdline, domain: bool) -> Cow<'static, str> {
    let (long_arg, default_config) = if domain {
        (DOMAIN_CONFIG, DEFAULT_DOMAIN_XML_FILE)
    } else {
        (SERVER_CONFIG, DEFAULT_STANDALONE_XML_FILE)
    };

    let mut iter = cmdline.args();
    while let Some(arg) = iter.next() {
        if !arg.starts_with(SERVER_CONFIG_SHORT) && !arg.starts_with(long_arg) {
            continue;
        }

        // Check if separated by '='
        if let Some((_, val)) = arg.split_once('=') {
            return val.to_string().into();
        }

        // Check if separated by space (next arg)
        if let Some(next_arg) = iter.next() {
            return next_arg.to_string().into();
        }
    }

    default_config.into()
}

fn find_server_group(domain_fs: &SubDirFs, server_name: &str) -> Result<String, Error> {
    let host_xml_path = Path::new(CONFIG_DIR).join(HOST_XML_FILE);
    let file = domain_fs.open(&host_xml_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let host: HostXML = quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse host.xml: {}", e)))?;

    for server in host.servers.server {
        if server.name == server_name {
            return Ok(server.group);
        }
    }
    Err(Error::MissingConfig(format!(
        "Server '{}' not found in host.xml",
        server_name
    )))
}

fn standalone_find_deployments(
    base_fs: &SubDirFs,
    config_file: &str,
) -> Result<Vec<ServerDeployment>, Error> {
    let config_path = Path::new(CONFIG_DIR).join(config_file);
    let file = base_fs.open(&config_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let descriptor: StandaloneXML = quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse {}: {}", config_file, e)))?;

    let result: Vec<_> = descriptor
        .deployments
        .deployment
        .into_iter()
        .filter(|d| xml_string_to_bool(d.enabled.as_deref()))
        .collect();

    Ok(result)
}

fn domain_find_deployments(
    base_fs: &SubDirFs,
    config_file: &str,
    server_name: &str,
) -> Result<Vec<ServerDeployment>, Error> {
    // Find the server group for this server
    let server_group = find_server_group(base_fs, server_name)?;

    // Parse domain.xml
    let config_path = Path::new(CONFIG_DIR).join(config_file);
    let file = base_fs.open(&config_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let descriptor: DomainXML = quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse {}: {}", config_file, e)))?;

    // Find the matching server group
    let current_group = descriptor
        .server_groups
        .server_group
        .iter()
        .find(|g| g.name == server_group)
        .ok_or_else(|| {
            Error::MissingConfig(format!(
                "Server group '{}' not found in domain.xml",
                server_group
            ))
        })?;

    // Index top-level deployments by name for faster lookup
    let indexed: HashMap<&str, &ServerDeployment> = descriptor
        .deployments
        .deployment
        .iter()
        .map(|d| (d.name.as_str(), d))
        .collect();

    // Filter to only include enabled deployments from this server group
    let result: Vec<_> = current_group
        .deployments
        .deployment
        .iter()
        .filter(|d| xml_string_to_bool(d.enabled.as_deref()))
        .filter_map(|d| indexed.get(d.name.as_str()).copied().cloned())
        .collect();

    Ok(result)
}

pub fn find_deployed_apps(
    cmdline: &Cmdline,
    ctx: &DetectionContext,
    domain_home: &Path,
) -> Result<Vec<Deployment>, Error> {
    // Verify we have jboss.home.dir (required property)
    let jboss_home = extract_java_property_from_args(cmdline, HOME_DIR_SYS_PROP)
        .ok_or_else(|| Error::MissingConfig("jboss.home.dir property not found".to_string()))?;
    let jboss_home = PathBuf::from(jboss_home);
    let jboss_home = ctx
        .resolve_working_dir_relative_path(&jboss_home)
        .unwrap_or(jboss_home);

    let (server_name, domain_mode) = extract_server_name(cmdline);
    if domain_mode && server_name.is_none() {
        return Err(Error::MissingConfig(
            "Server name not found in domain mode".to_string(),
        ));
    }

    let config_file = extract_config_filename(cmdline, domain_mode);

    let deployments = if domain_mode {
        let base_path = jboss_home.join(DOMAIN_BASE);
        let base_fs = ctx.fs.sub(&base_path)?;
        domain_find_deployments(
            &base_fs,
            &config_file,
            server_name.as_ref().ok_or_else(|| {
                Error::MissingConfig("Server name required for domain mode".to_string())
            })?,
        )?
    } else {
        let base_path = jboss_home.join(STANDALONE_BASE);
        let base_fs = ctx.fs.sub(&base_path)?;
        standalone_find_deployments(&base_fs, &config_file)?
    };

    let result: Vec<_> = deployments
        .into_iter()
        .filter_map(|d| {
            let content = d.content?;
            let hash = &content.hash;
            let path = domain_home
                .join(DATA_DIR)
                .join(CONTENT_DIR)
                .join(hash.get(0..2)?)
                .join(hash.get(2..)?)
                .join(CONTENT_DIR);

            Some(Deployment {
                name: d.runtime_name,
                path,
                kind: None,
                context_root: None,
            })
        })
        .collect();

    Ok(result)
}

pub fn custom_extract_war_context_root(deployment_fs: &mut DeploymentFs) -> Option<String> {
    // Try WEB-INF first, then META-INF
    for xml_file in [WEB_XML_FILE_WEB_INF, WEB_XML_FILE_META_INF] {
        let xml_path = Path::new(xml_file);
        if let Ok(buf) = deployment_fs.read_file_to_vec(xml_path)
            && let Ok(jboss_web) = quick_xml::de::from_reader::<_, WebXML>(buf.as_slice())
            && !jboss_web.context_root.is_empty()
        {
            return Some(jboss_web.context_root);
        }
    }
    None
}

/// Parse string as boolean (defaults to true if empty/missing)
fn xml_string_to_bool(s: Option<&str>) -> bool {
    match s {
        None => true,
        Some(s) => !matches!(s.to_lowercase().as_str(), "0" | "false"),
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
#[allow(clippy::indexing_slicing)]
#[allow(clippy::panic)]
mod tests {
    use super::*;

    use crate::fs::SubDirFs;

    #[test]
    fn test_extract_server_name() {
        struct TestCase {
            name: &'static str,
            args: Vec<&'static str>,
            expected: &'static str,
            domain_mode: bool,
        }

        let tests = vec![
            TestCase {
                name: "server name present",
                args: vec!["java", "-D[Server:server1]"],
                expected: "server1",
                domain_mode: true,
            },
            TestCase {
                name: "server name absent",
                args: vec!["java", "-D[Standalone]"],
                expected: "",
                domain_mode: false,
            },
            TestCase {
                name: "invalid server name",
                args: vec!["java", "-D[Server:"],
                expected: "",
                domain_mode: true,
            },
        ];

        for test in tests {
            let cmdline = crate::procfs::Cmdline::from(&test.args[..]);
            let (value, domain_mode) = extract_server_name(&cmdline);
            let value_str = value.as_deref().unwrap_or("");
            assert_eq!(
                value_str, test.expected,
                "test case '{}' failed: expected '{}', got '{}'",
                test.name, test.expected, value_str
            );
            assert_eq!(
                domain_mode, test.domain_mode,
                "test case '{}' failed: domain_mode should be {}",
                test.name, test.domain_mode
            );
        }
    }

    #[test]
    fn test_extract_config_filename() {
        let tests: Vec<(&str, Vec<&str>, bool, &str)> = vec![
            ("default for standalone", vec![], false, "standalone.xml"),
            (
                "standalone with short option",
                vec![
                    "java",
                    "-jar",
                    "jboss-modules.jar",
                    "-c",
                    "standalone-ha.xml",
                ],
                false,
                "standalone-ha.xml",
            ),
            (
                "standalone with long option",
                vec![
                    "java",
                    "-jar",
                    "jboss-modules.jar",
                    "--server-config=standalone-full.xml",
                ],
                false,
                "standalone-full.xml",
            ),
            ("default for domain", vec![], true, "domain.xml"),
            (
                "domain with short option",
                vec!["java", "-jar", "jboss-modules.jar", "-c", "domain-ha.xml"],
                true,
                "domain-ha.xml",
            ),
            (
                "domain with long option",
                vec![
                    "java",
                    "-jar",
                    "jboss-modules.jar",
                    "--domain-config=domain-full.xml",
                ],
                true,
                "domain-full.xml",
            ),
        ];

        for (name, args, domain, expected) in tests {
            let cmdline = crate::procfs::Cmdline::from(&args[..]);
            let result = extract_config_filename(&cmdline, domain);
            assert_eq!(result, expected, "test case '{}' failed", name);
        }
    }

    #[test]
    fn test_custom_extract_war_context_root() {
        use super::super::DeploymentFs;
        use std::fs;
        use tempfile::TempDir;

        struct TestCase {
            name: &'static str,
            jboss_web_xml: &'static str,
            location: &'static str,
            expected: &'static str,
        }

        let tests = vec![
            TestCase {
                name: "jboss-web in META-INF",
                jboss_web_xml: r#"<?xml version="1.0" encoding="UTF-8"?>
<jboss-web version="7.1" xmlns="http://www.jboss.com/xml/ns/javaee" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:schemaLocation="http://www.jboss.com/xml/ns/javaee http://www.jboss.org/schema/jbossas/jboss-web_7_1.xsd">
    <context-root>/myapp</context-root>
</jboss-web>"#,
                location: "META-INF/jboss-web.xml",
                expected: "/myapp",
            },
            TestCase {
                name: "jboss-web in WEB-INF",
                jboss_web_xml: "<jboss-web><context-root>/yourapp</context-root></jboss-web>",
                location: "WEB-INF/jboss-web.xml",
                expected: "/yourapp",
            },
            TestCase {
                name: "jboss-web in WEB-INF without context-root",
                jboss_web_xml: "<jboss-web/>",
                location: "WEB-INF/jboss-web.xml",
                expected: "",
            },
            TestCase {
                name: "jboss-web missing",
                jboss_web_xml: "",
                location: "",
                expected: "",
            },
        ];

        for test in tests {
            let tmp_dir = TempDir::new().unwrap();

            // Create the file if location is specified
            if !test.location.is_empty() {
                let file_path = tmp_dir.path().join(test.location);
                if let Some(parent) = file_path.parent() {
                    fs::create_dir_all(parent).unwrap();
                }
                fs::write(&file_path, test.jboss_web_xml).unwrap();
            }

            let subdir_fs = SubDirFs::new(tmp_dir.path()).unwrap();
            let mut deployment_fs = DeploymentFs::Directory(subdir_fs);
            let result = custom_extract_war_context_root(&mut deployment_fs);

            let result_str = result.as_deref().unwrap_or("");
            assert_eq!(
                result_str, test.expected,
                "test case '{}' failed: expected '{}', got '{}'",
                test.name, test.expected, result_str
            );
        }
    }

    #[test]
    fn test_find_server_group() {
        use std::fs;
        use tempfile::TempDir;

        struct TestCase {
            name: &'static str,
            server_name: &'static str,
            host_xml: &'static str,
            expected: Option<&'static str>,
        }

        let host_xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<host xmlns="urn:jboss:domain:20.0" name="primary">
    <servers>
        <server name="server-one" group="main-server-group"/>
        <server name="server-two" group="main-server-group" auto-start="true">
            <jvm name="default"/>
            <socket-bindings port-offset="150"/>
        </server>
        <server name="server-three" group="other-server-group" auto-start="false">
            <jvm name="default"/>
            <socket-bindings port-offset="250"/>
        </server>
    </servers>
</host>"#;

        let tests = vec![
            TestCase {
                name: "server group found",
                server_name: "server-two",
                host_xml,
                expected: Some("main-server-group"),
            },
            TestCase {
                name: "server group not found",
                server_name: "server-four",
                host_xml,
                expected: None,
            },
            TestCase {
                name: "empty host.xml",
                server_name: "server-one",
                host_xml: "",
                expected: None,
            },
        ];

        for test in tests {
            let tmp_dir = TempDir::new().unwrap();
            let config_dir = tmp_dir.path().join("configuration");
            fs::create_dir(&config_dir).unwrap();

            // Only write the file if host_xml is not empty
            if !test.host_xml.is_empty() {
                let host_xml_path = config_dir.join("host.xml");
                fs::write(&host_xml_path, test.host_xml).unwrap();
            }

            let domain_fs = SubDirFs::new(tmp_dir.path()).unwrap();

            let result = find_server_group(&domain_fs, test.server_name);
            match test.expected {
                Some(expected_group) => {
                    assert!(
                        result.is_ok(),
                        "test case '{}' failed: expected Ok, got Err: {:?}",
                        test.name,
                        result
                    );
                    assert_eq!(
                        result.unwrap(),
                        expected_group,
                        "test case '{}' failed",
                        test.name
                    );
                }
                None => {
                    assert!(
                        result.is_err(),
                        "test case '{}' failed: expected Err, got Ok: {:?}",
                        test.name,
                        result
                    );
                }
            }
        }
    }

    #[test]
    fn test_find_deployed_apps() {
        use super::super::tests::ErrorChecker;
        use crate::service_name::context::DetectionContext;
        use crate::test_utils::TestDataFs;
        use std::collections::HashMap;
        use std::fs;
        use tempfile::TempDir;

        #[derive(Clone)]
        struct ExpectedDeployment {
            name: String,
            path: String,
        }

        struct TestCase {
            name: &'static str,
            args: Vec<String>,
            domain_home: Option<String>,
            expected: Vec<ExpectedDeployment>,
            use_real_fs: bool,
            setup_temp_fs: Option<fn(&TempDir)>,
            expected_error: Option<ErrorChecker>,
        }

        let jboss_test_app_root = "../sub";
        let jboss_test_app_root_absolute = "/sub";

        let tests = vec![
            TestCase {
                name: "standalone",
                args: vec![format!("-Djboss.home.dir={}", jboss_test_app_root)],
                domain_home: Some(format!("{}/standalone", jboss_test_app_root_absolute)),
                expected: vec![
                    ExpectedDeployment {
                        name: "app.ear".to_string(),
                        path: format!(
                            "{}/standalone/data/content/38/e/content",
                            jboss_test_app_root_absolute
                        ),
                    },
                    ExpectedDeployment {
                        name: "web3.war".to_string(),
                        path: format!(
                            "{}/standalone/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content",
                            jboss_test_app_root_absolute
                        ),
                    },
                    ExpectedDeployment {
                        name: "web4.war".to_string(),
                        path: format!(
                            "{}/standalone/data/content/f0/c/content",
                            jboss_test_app_root_absolute
                        ),
                    },
                ],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: None,
            },
            TestCase {
                name: "standalone - missing home",
                args: vec![],
                domain_home: None,
                expected: vec![],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "standalone - missing config",
                args: vec!["-Djboss.home.dir=jboss".to_string()],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/standalone/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::Io(_)),
                        "Expected JeeError::Io but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "standalone - bad config",
                args: vec!["-Djboss.home.dir=jboss".to_string()],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_file = tmp_dir
                        .path()
                        .join("jboss/standalone/configuration/standalone.xml");
                    fs::create_dir_all(config_file.parent().unwrap()).unwrap();
                    fs::write(&config_file, "evil").unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::XmlParse(_)),
                        "Expected JeeError::XmlParse but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "domain - main server group",
                args: vec![
                    format!("-Djboss.home.dir={}", jboss_test_app_root),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: Some(format!("{}/domain/servers/server-one", jboss_test_app_root)),
                expected: vec![
                    ExpectedDeployment {
                        name: "app.ear".to_string(),
                        path: format!(
                            "{}/domain/servers/server-one/data/content/38/e/content",
                            jboss_test_app_root
                        ),
                    },
                    ExpectedDeployment {
                        name: "web3.war".to_string(),
                        path: format!(
                            "{}/domain/servers/server-one/data/content/8b/e62d23ec32e3956fecf9b5c35e8405510a825f/content",
                            jboss_test_app_root
                        ),
                    },
                    ExpectedDeployment {
                        name: "web4.war".to_string(),
                        path: format!(
                            "{}/domain/servers/server-one/data/content/f0/c/content",
                            jboss_test_app_root
                        ),
                    },
                ],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: None,
            },
            TestCase {
                name: "domain- other server group",
                args: vec![
                    format!("-Djboss.home.dir={}", jboss_test_app_root),
                    "-D[Server:server-three]".to_string(),
                ],
                domain_home: Some(format!(
                    "{}/domain/servers/server-three",
                    jboss_test_app_root
                )),
                expected: vec![ExpectedDeployment {
                    name: "web4.war".to_string(),
                    path: format!(
                        "{}/domain/servers/server-three/data/content/f0/c/content",
                        jboss_test_app_root
                    ),
                }],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: None,
            },
            TestCase {
                name: "domain- server not found",
                args: vec![
                    format!("-Djboss.home.dir={}", jboss_test_app_root),
                    "-D[Server:server-four]".to_string(),
                ],
                domain_home: Some(format!(
                    "{}/domain/servers/server-four",
                    jboss_test_app_root
                )),
                expected: vec![],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "domain- malformed server",
                args: vec![
                    format!("-Djboss.home.dir={}", jboss_test_app_root),
                    "-D[Server:]".to_string(),
                ],
                domain_home: Some(format!(
                    "{}/domain/servers/server-four",
                    jboss_test_app_root
                )),
                expected: vec![],
                use_real_fs: true,
                setup_temp_fs: None,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "domain- missing dir",
                args: vec![
                    "-Djboss.home.dir=jboss".to_string(),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::Io(_)),
                        "Expected JeeError::Io but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "domain- missing domain.xml",
                args: vec![
                    "-Djboss.home.dir=jboss".to_string(),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/domain/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                    // Create host.xml but NOT domain.xml
                    let host_xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<host xmlns="urn:jboss:domain:20.0" name="primary">
    <servers>
        <server name="server-one" group="main-server-group"/>
    </servers>
</host>"#;
                    fs::write(config_dir.join("host.xml"), host_xml).unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| match err {
                    Error::Io(io_err) => {
                        assert_eq!(
                            io_err.kind(),
                            std::io::ErrorKind::NotFound,
                            "Expected NotFound error for missing domain.xml but got {:?}",
                            io_err.kind()
                        );
                    }
                    _ => panic!("Expected JeeError::Io but got {:?}", err),
                })),
            },
            TestCase {
                name: "domain- missing files",
                args: vec![
                    "-Djboss.home.dir=jboss".to_string(),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/domain/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                    // Create domain.xml but NOT host.xml
                    let domain_xml = r#"<?xml version='1.0' encoding='UTF-8'?>
<domain xmlns="urn:jboss:domain:1.3">
    <server-groups>
        <server-group name="main-server-group" profile="default">
        </server-group>
    </server-groups>
</domain>"#;
                    fs::write(config_dir.join("domain.xml"), domain_xml).unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| match err {
                    Error::Io(io_err) => {
                        assert_eq!(
                            io_err.kind(),
                            std::io::ErrorKind::NotFound,
                            "Expected NotFound error for missing host.xml but got {:?}",
                            io_err.kind()
                        );
                    }
                    _ => panic!("Expected JeeError::Io but got {:?}", err),
                })),
            },
            TestCase {
                name: "domain- broken domain.xml",
                args: vec![
                    "-Djboss.home.dir=jboss".to_string(),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/domain/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                    // Create valid host.xml
                    let host_xml = r#"<?xml version="1.0" encoding="UTF-8"?>
<host xmlns="urn:jboss:domain:20.0" name="primary">
    <servers>
        <server name="server-one" group="main-server-group"/>
    </servers>
</host>"#;
                    fs::write(config_dir.join("host.xml"), host_xml).unwrap();
                    // Create broken domain.xml
                    fs::write(config_dir.join("domain.xml"), "evil").unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::XmlParse(_)),
                        "Expected JeeError::XmlParse but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "domain- broken host.xml",
                args: vec![
                    "-Djboss.home.dir=jboss".to_string(),
                    "-D[Server:server-one]".to_string(),
                ],
                domain_home: None,
                expected: vec![],
                use_real_fs: false,
                setup_temp_fs: Some(|tmp_dir| {
                    let config_dir = tmp_dir.path().join("jboss/domain/configuration");
                    fs::create_dir_all(&config_dir).unwrap();
                    // Create valid domain.xml
                    let domain_xml = r#"<?xml version='1.0' encoding='UTF-8'?>
<domain xmlns="urn:jboss:domain:1.3">
    <server-groups>
        <server-group name="main-server-group" profile="default">
        </server-group>
    </server-groups>
</domain>"#;
                    fs::write(config_dir.join("domain.xml"), domain_xml).unwrap();
                    // Create broken host.xml
                    fs::write(config_dir.join("host.xml"), "broken").unwrap();
                }),
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::XmlParse(_)),
                        "Expected JeeError::XmlParse but got {:?}",
                        err
                    );
                })),
            },
        ];

        for test in tests {
            let (envs, fs) = if test.use_real_fs {
                // Use real filesystem for tests that reference actual testdata
                let fs = TestDataFs::new("jee/jboss");
                let mut envs = HashMap::new();
                envs.insert("PWD".to_string(), "/sibling".to_string());
                (envs, fs)
            } else {
                // Use temporary filesystem for error cases
                let fs = TestDataFs::new_empty();
                if let Some(setup) = test.setup_temp_fs {
                    setup(fs.as_ref());
                }
                (HashMap::new(), fs)
            };
            let ctx = DetectionContext::new(1, envs, fs.as_ref());

            let domain_home = test.domain_home.as_deref().unwrap_or(".");
            let args_strs: Vec<&str> = test.args.iter().map(|s| s.as_str()).collect();
            let cmdline = crate::procfs::Cmdline::from(&args_strs[..]);
            let result = find_deployed_apps(&cmdline, &ctx, std::path::Path::new(domain_home));

            if test.expected.is_empty() && test.expected_error.is_some() {
                assert!(
                    result.is_err(),
                    "test case '{}' failed: expected error but got {:?}",
                    test.name,
                    result
                );

                if let Some(check_error) = test.expected_error {
                    let err = result.unwrap_err();
                    check_error(&err);
                }
            } else if test.expected.is_empty() {
                assert!(
                    result.is_ok() && result.as_ref().unwrap().is_empty() || result.is_err(),
                    "test case '{}' failed: expected empty or error, got {:?}",
                    test.name,
                    result
                );
            } else {
                assert!(
                    result.is_ok(),
                    "test case '{}' failed: expected Ok, got Err: {:?}",
                    test.name,
                    result
                );
                let deployments = result.unwrap();
                assert_eq!(
                    deployments.len(),
                    test.expected.len(),
                    "test case '{}' failed: expected {} deployments, got {}",
                    test.name,
                    test.expected.len(),
                    deployments.len()
                );

                // Check deployment names and paths
                for expected in &test.expected {
                    let found = deployments.iter().find(|d| d.name == expected.name);
                    assert!(
                        found.is_some(),
                        "test case '{}' failed: deployment '{}' not found",
                        test.name,
                        expected.name
                    );
                    let deployment = found.unwrap();
                    assert_eq!(
                        deployment.path.to_string_lossy().as_ref(),
                        expected.path,
                        "test case '{}' failed: path mismatch for '{}'",
                        test.name,
                        expected.name
                    );
                }
            }
        }
    }
}
