// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use super::{Deployment, DeploymentType, Error};
use crate::procfs::Cmdline;
use crate::service_name::context::DetectionContext;
use serde::Deserialize;
use std::io::BufReader;
use std::path::Path;

/// WebSphere app deployment descriptor
#[derive(Debug, Deserialize)]
struct AppDeployment {
    #[serde(rename = "deployedObject", default)]
    deployed_object: DeployedObject,
    #[serde(rename = "deploymentTargets", default)]
    deployment_targets: Vec<DeploymentTarget>,
}

#[derive(Debug, Deserialize, Default)]
struct DeployedObject {
    #[serde(rename = "targetMappings", default)]
    target_mappings: Vec<TargetMapping>,
}

/// Target mapping for deployments
#[derive(Debug, Deserialize)]
struct TargetMapping {
    #[serde(rename = "@enable", default)]
    enable: bool,
    #[serde(rename = "@target")]
    server_target: String,
}

/// Deployment target information
#[derive(Debug, Deserialize)]
struct DeploymentTarget {
    #[serde(rename = "@id")]
    id: String,
    #[serde(rename = "@name")]
    server_name: String,
    #[serde(rename = "@nodeName")]
    node_name: String,
}

fn is_application_deployed(
    ctx: &DetectionContext,
    descriptor_path: &Path,
    node_name: &str,
    server_name: &str,
) -> Result<bool, Error> {
    let file = ctx.fs.open(descriptor_path)?;
    let reader = BufReader::new(file.verify(None)?);
    let app_deployment: AppDeployment = quick_xml::de::from_reader(reader)
        .map_err(|e| Error::XmlParse(format!("Failed to parse deployment.xml: {}", e)))?;

    // Find matching target
    let matching_target = app_deployment
        .deployment_targets
        .iter()
        .find(|t| t.node_name == node_name && t.server_name == server_name)
        .map(|t| &t.id)
        .ok_or_else(|| {
            Error::MissingConfig(format!(
                "No deployment target found for node '{}' and server '{}'",
                node_name, server_name
            ))
        })?;

    // Check if any enabled mapping references this target
    let is_deployed = app_deployment
        .deployed_object
        .target_mappings
        .iter()
        .any(|m| m.enable && &m.server_target == matching_target);

    Ok(is_deployed)
}

pub fn find_deployed_apps(
    cmdline: &Cmdline,
    ctx: &DetectionContext,
    domain_home: &Path,
) -> Result<Vec<Deployment>, Error> {
    // Get the last 3 args: cell_name, node_name, server_name
    // WebSphere passes these as the last 3 command-line arguments
    let mut args = cmdline.args();

    let server_name = args
        .next_back()
        .ok_or_else(|| Error::MissingConfig("Server name is empty".to_string()))?;
    let node_name = args
        .next_back()
        .ok_or_else(|| Error::MissingConfig("Node name is empty".to_string()))?;
    let cell_name = args
        .next_back()
        .ok_or_else(|| Error::MissingConfig("Cell name is empty".to_string()))?;

    if cell_name.is_empty() || node_name.is_empty() || server_name.is_empty() {
        return Err(Error::MissingConfig(
            "Cell name, node name, or server name is empty".to_string(),
        ));
    }

    // Find deployment.xml files matching the pattern:
    // {domainHome}/config/cells/{cellName}/applications/*/deployments/*/deployment.xml
    let base_path = domain_home
        .join("config")
        .join("cells")
        .join(cell_name)
        .join("applications");

    let mut apps = Vec::new();

    // Walk directory tree with filter_entry to skip directories that don't match the pattern
    // Pattern: applications/{appName}/deployments/{deploymentName}/deployment.xml
    // Depths:  0            1         2            3                4
    for entry in ctx
        .fs
        .walker(&base_path.to_string_lossy())
        .into_iter()
        .filter_entry(|e| {
            let depth = e.depth();
            if depth == 0 {
                true
            } else if depth == 1 {
                e.file_type().is_dir()
            } else if depth == 2 {
                e.file_type().is_dir() && e.file_name() == "deployments"
            } else if depth == 3 {
                e.file_type().is_dir()
            } else {
                e.file_type().is_file() && e.file_name() == "deployment.xml"
            }
        })
        .filter_map(Result::ok)
    {
        // Only process deployment.xml files at the correct depth (filter_entry ensures correct structure)
        if entry.depth() != 4
            || !entry.file_type().is_file()
            || entry.file_name() != "deployment.xml"
        {
            continue;
        }

        // Get path relative to SubDirFs root
        let Some(path_str) = ctx.fs.make_relative(entry.path()) else {
            continue;
        };

        let path = std::path::PathBuf::from(&path_str);

        // Errors in checking specific deployments are logged but don't fail the whole function
        match is_application_deployed(ctx, &path, node_name, server_name) {
            Ok(true) => {
                // The deployment path is the directory containing deployment.xml
                if let Some(parent) = path.parent() {
                    apps.push(Deployment {
                        name: String::new(),
                        path: parent.to_path_buf(),
                        kind: Some(DeploymentType::Ear),
                        context_root: None,
                    });
                }
            }
            Ok(false) => {
                // Deployment not enabled for this server
            }
            Err(e) => {
                log::debug!(
                    "websphere::find_deployed_apps: error checking deployment at {:?}: {}",
                    path,
                    e
                );
            }
        }
    }

    Ok(apps)
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
    use std::path::PathBuf;
    use tempfile::TempDir;

    #[test]
    fn test_default_context_root_from_file() {
        let result = super::super::standard_extract_context_from_war_name("myapp.war");
        assert_eq!(result, Some("myapp".to_string()));
    }

    #[test]
    fn test_find_deployed_apps() {
        use super::super::tests::ErrorChecker;

        struct TestCase {
            name: &'static str,
            args: Vec<&'static str>,
            deployment_xml: Option<&'static str>,
            expected_count: usize,
            expected_error: Option<ErrorChecker>,
        }

        let tests = vec![
            TestCase {
                name: "find enabled deployment with 2 servers",
                args: vec!["cell1", "node1", "server1"],
                deployment_xml: Some(
                    r#"
<appdeployment:Deployment xmi:version="2.0" xmlns:xmi="http://www.omg.org/XMI" xmlns:appdeployment="http://www.ibm.com/websphere/appserver/schemas/5.0/appdeployment.xmi" xmi:id="Deployment_1710254881381">
    <deployedObject xmi:type="appdeployment:ApplicationDeployment" xmi:id="ApplicationDeployment_1710254881381" deploymentId="0" startingWeight="1" binariesURL="$(APP_INSTALL_ROOT)/DefaultCell01/sample.ear" useMetadataFromBinaries="false" enableDistribution="true" createMBeansForResources="true" reloadEnabled="false" appContextIDForSecurity="href:DefaultCell01/sample_ear" filePermission=".*\.dll=755#.*\.so=755#.*\.a=755#.*\.sl=755" allowDispatchRemoteInclude="false" allowServiceRemoteInclude="false" asyncRequestDispatchType="DISABLED" standaloneModule="true" enableClientModule="false">
        <targetMappings xmi:id="DeploymentTargetMapping_1710254881382" enable="true" target="ServerTarget_1710254881382"/>
        <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" target="ServerTarget_1710254881383"/>
        <classloader xmi:id="Classloader_1710254881382" mode="PARENT_FIRST"/>
        <modules xmi:type="appdeployment:WebModuleDeployment" xmi:id="WebModuleDeployment_1710254881382" deploymentId="1" startingWeight="10000" uri="sample.war" containsEJBContent="0">
            <targetMappings xmi:id="DeploymentTargetMapping_1710254881382" target="ServerTarget_1710254881382"/>
            <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" target="ServerTarget_1710254881383"/>
            <classloader xmi:id="Classloader_1710254881383"/>
        </modules>
        <properties xmi:id="Property_1710254881382" name="metadata.complete" value="true"/>
    </deployedObject>
    <deploymentTargets xmi:type="appdeployment:ServerTarget" xmi:id="ServerTarget_1710254881382" name="server1" nodeName="node1"/>
    <deploymentTargets xmi:type="appdeployment:ServerTarget" xmi:id="ServerTarget_1710254881383" name="server2" nodeName="node1"/>
</appdeployment:Deployment>
"#,
                ),
                expected_count: 1,
                expected_error: None,
            },
            TestCase {
                name: "skip disabled deployment - 2 servers",
                args: vec!["cell1", "node1", "server1"],
                deployment_xml: Some(
                    r#"
<appdeployment:Deployment xmi:version="2.0" xmlns:xmi="http://www.omg.org/XMI" xmlns:appdeployment="http://www.ibm.com/websphere/appserver/schemas/5.0/appdeployment.xmi" xmi:id="Deployment_1710254881381">
    <deployedObject xmi:type="appdeployment:ApplicationDeployment" xmi:id="ApplicationDeployment_1710254881381" deploymentId="0" startingWeight="1" binariesURL="$(APP_INSTALL_ROOT)/DefaultCell01/sample.ear" useMetadataFromBinaries="false" enableDistribution="true" createMBeansForResources="true" reloadEnabled="false" appContextIDForSecurity="href:DefaultCell01/sample_ear" filePermission=".*\.dll=755#.*\.so=755#.*\.a=755#.*\.sl=755" allowDispatchRemoteInclude="false" allowServiceRemoteInclude="false" asyncRequestDispatchType="DISABLED" standaloneModule="true" enableClientModule="false">
        <targetMappings xmi:id="DeploymentTargetMapping_1710254881382" target="ServerTarget_1710254881382"/>
        <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" enable="true" target="ServerTarget_1710254881383"/>
        <classloader xmi:id="Classloader_1710254881382" mode="PARENT_FIRST"/>
        <modules xmi:type="appdeployment:WebModuleDeployment" xmi:id="WebModuleDeployment_1710254881382" deploymentId="1" startingWeight="10000" uri="sample.war" containsEJBContent="0">
            <targetMappings xmi:id="DeploymentTargetMapping_1710254881382" target="ServerTarget_1710254881382"/>
            <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" target="ServerTarget_1710254881383"/>
            <classloader xmi:id="Classloader_1710254881383"/>
        </modules>
        <properties xmi:id="Property_1710254881382" name="metadata.complete" value="true"/>
    </deployedObject>
    <deploymentTargets xmi:type="appdeployment:ServerTarget" xmi:id="ServerTarget_1710254881382" name="server1" nodeName="node1"/>
    <deploymentTargets xmi:type="appdeployment:ServerTarget" xmi:id="ServerTarget_1710254881383" name="server2" nodeName="node1"/>
</appdeployment:Deployment>
"#,
                ),
                expected_count: 0,
                expected_error: None,
            },
            TestCase {
                name: "not matching server",
                args: vec!["cell1", "node1", "server1"],
                deployment_xml: Some(
                    r#"
<appdeployment:Deployment xmi:version="2.0" xmlns:xmi="http://www.omg.org/XMI" xmlns:appdeployment="http://www.ibm.com/websphere/appserver/schemas/5.0/appdeployment.xmi" xmi:id="Deployment_1710254881381">
    <deployedObject xmi:type="appdeployment:ApplicationDeployment" xmi:id="ApplicationDeployment_1710254881381" deploymentId="0" startingWeight="1" binariesURL="$(APP_INSTALL_ROOT)/DefaultCell01/sample.ear" useMetadataFromBinaries="false" enableDistribution="true" createMBeansForResources="true" reloadEnabled="false" appContextIDForSecurity="href:DefaultCell01/sample_ear" filePermission=".*\.dll=755#.*\.so=755#.*\.a=755#.*\.sl=755" allowDispatchRemoteInclude="false" allowServiceRemoteInclude="false" asyncRequestDispatchType="DISABLED" standaloneModule="true" enableClientModule="false">
        <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" enable="true" target="ServerTarget_1710254881383"/>
        <classloader xmi:id="Classloader_1710254881382" mode="PARENT_FIRST"/>
        <modules xmi:type="appdeployment:WebModuleDeployment" xmi:id="WebModuleDeployment_1710254881382" deploymentId="1" startingWeight="10000" uri="sample.war" containsEJBContent="0">
            <targetMappings xmi:id="DeploymentTargetMapping_1710254881383" target="ServerTarget_1710254881383"/>
            <classloader xmi:id="Classloader_1710254881383"/>
        </modules>
        <properties xmi:id="Property_1710254881382" name="metadata.complete" value="true"/>
    </deployedObject>
    <deploymentTargets xmi:type="appdeployment:ServerTarget" xmi:id="ServerTarget_1710254881383" name="server2" nodeName="node1"/>
</appdeployment:Deployment>
"#,
                ),
                expected_count: 0,
                expected_error: None,
            },
            TestCase {
                name: "no deployment file",
                args: vec!["cell1", "node1", "server1"],
                deployment_xml: None,
                expected_count: 0,
                expected_error: None,
            },
            TestCase {
                name: "missing server name",
                args: vec!["", "node1"],
                deployment_xml: None,
                expected_count: 0,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "empty nodename",
                args: vec!["cell1", "", "server1"],
                deployment_xml: None,
                expected_count: 0,
                expected_error: Some(Box::new(|err: &Error| {
                    assert!(
                        matches!(err, Error::MissingConfig(_)),
                        "Expected JeeError::MissingConfig but got {:?}",
                        err
                    );
                })),
            },
            TestCase {
                name: "bad deployment xml",
                args: vec!["cell1", "node1", "server1"],
                deployment_xml: Some("bad"),
                expected_count: 0,
                expected_error: None,
            },
        ];

        for tt in tests {
            let tmp_dir = TempDir::new().unwrap();
            let base = tmp_dir.path().join("base");

            // Create deployment.xml if provided
            if let Some(xml_content) = tt.deployment_xml {
                let cell_name = if tt.args.is_empty() {
                    "cell1"
                } else {
                    tt.args[0]
                };
                let deployment_dir = base.join(format!(
                    "config/cells/{}/applications/myapp.ear/deployments/myapp",
                    cell_name
                ));
                fs::create_dir_all(&deployment_dir).unwrap();
                fs::write(deployment_dir.join("deployment.xml"), xml_content).unwrap();
            }

            let fs_root = SubDirFs::new(tmp_dir.path()).unwrap();
            let envs = HashMap::new();
            let ctx = DetectionContext::new(1, envs, &fs_root);
            let args: Vec<String> = tt.args.iter().map(|s| s.to_string()).collect();

            let args_strs: Vec<&str> = args.iter().map(|s| s.as_str()).collect();
            let cmdline = crate::procfs::Cmdline::from(&args_strs[..]);
            let result = find_deployed_apps(&cmdline, &ctx, Path::new("base"));

            // Check if we expect an error (empty or too few args)
            let should_error = tt.args.len() < 3 || tt.args.iter().any(|s| s.is_empty());

            if should_error {
                assert!(
                    result.is_err(),
                    "{}: expected error but got {:?}",
                    tt.name,
                    result
                );

                // Verify error variant if callback provided
                if let Some(check_error) = tt.expected_error {
                    let err = result.unwrap_err();
                    check_error(&err);
                }
            } else {
                assert!(
                    result.is_ok(),
                    "{}: expected success but got error: {:?}",
                    tt.name,
                    result.err()
                );
                let deployments = result.unwrap();
                assert_eq!(
                    deployments.len(),
                    tt.expected_count,
                    "{}: expected {} deployments, got {}",
                    tt.name,
                    tt.expected_count,
                    deployments.len()
                );

                // Verify expected path format if deployment was found
                if tt.expected_count > 0 {
                    assert_eq!(
                        deployments[0].path,
                        PathBuf::from(
                            "base/config/cells/cell1/applications/myapp.ear/deployments/myapp"
                        ),
                        "{}: unexpected deployment path",
                        tt.name
                    );
                    assert_eq!(
                        deployments[0].kind,
                        Some(DeploymentType::Ear),
                        "{}: expected EAR deployment type",
                        tt.name
                    );
                }
            }
        }
    }
}
