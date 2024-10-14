// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	"path"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/envs"
)

func TestWebsphereFindDeployedApps(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		deploymentXML string
		expected      []jeeDeployment
	}{
		{
			name: "find enabled deployment with 2 servers ",
			args: []string{"cell1", "node1", "server1"},
			deploymentXML: `
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
`,
			expected: []jeeDeployment{{
				dt:   ear,
				path: "base/config/cells/cell1/applications/myapp.ear/deployments/myapp",
			}},
		},
		{
			name: "skip disabled deployment - 2 servers",
			args: []string{"cell1", "node1", "server1"},
			deploymentXML: `
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
`,
		},
		{
			name: "not matching server",
			args: []string{"cell1", "node1", "server1"},
			deploymentXML: `
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
`,
		},
		{
			name: "no deployment file",
			args: []string{"cell1", "node1", "server1"},
		},
		{
			name: "missing server name",
			args: []string{"", "node1"},
		},
		{
			name: "empty nodename",
			args: []string{"cell1", "", "server1"},
		},
		{
			name:          "bad deployment xml",
			args:          []string{"cell1", "node1", "server1"},
			deploymentXML: "bad",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := fstest.MapFS{}
			if len(tt.deploymentXML) > 0 {
				fs[path.Join("base", "config", "cells", tt.args[0], "applications", "myapp.ear", "deployments", "myapp", "deployment.xml")] =
					&fstest.MapFile{Data: []byte(tt.deploymentXML)}
			}

			value, ok := newWebsphereExtractor(NewDetectionContext(tt.args, envs.NewVariables(nil), fs)).findDeployedApps("base")
			require.Equal(t, tt.expected, value)
			require.Equal(t, len(value) > 0, ok)
		})
	}
}

func TestWebsphereDefaultContextRootFromFile(t *testing.T) {
	value, ok := newWebsphereExtractor(NewDetectionContext(nil, envs.NewVariables(nil), nil)).defaultContextRootFromFile("myapp.war")
	require.Equal(t, "myapp", value)
	require.True(t, ok)
}
