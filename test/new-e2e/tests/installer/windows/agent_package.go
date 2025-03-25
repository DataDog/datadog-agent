// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// AgentVersionManager provides Agent package information for a particular Agent version for the installer tests
type AgentVersionManager struct {
	// version should match Agent's version.AgentVersion field
	version agentVersion.Version
	// packageVersion should match Agent's version.AgentPackageVersion field
	packageVersion string
	ociPackage     TestPackageConfig
	msiPackage     *windowsagent.Package
}

// NewAgentVersionManager creates a new AgentVersionManager
func NewAgentVersionManager(versionStr, packageVersionStr string, ociPackage TestPackageConfig, msiPackage *windowsagent.Package) (*AgentVersionManager, error) {
	version, err := agentVersion.New(versionStr, "")
	if err != nil {
		return nil, err
	}
	return &AgentVersionManager{
		version:        version,
		packageVersion: packageVersionStr,
		ociPackage:     ociPackage,
		msiPackage:     msiPackage,
	}, nil
}

// Version returns the Agent version as returned by the version command, e.g. 7.60.0
//
// this should match the Agent's version.AgentVersion field
//
// Pipeline build example: 7.64.0-devel
func (avm *AgentVersionManager) Version() string {
	return avm.version.GetNumberAndPre()
}

// PackageVersion returns the Agent package version, e.g. 7.60.0-1
//
// this should match the Agent's version.AgentPackageVersion field
//
// Pipeline build example: 7.64.0-devel.git.1220.aaf8a1c.pipeline.58948204-1
func (avm *AgentVersionManager) PackageVersion() string {
	return avm.packageVersion
}

// OCIPackage returns the OCI package configuration
func (avm *AgentVersionManager) OCIPackage() TestPackageConfig {
	return avm.ociPackage
}

// MSIPackage returns the MSI package configuration
func (avm *AgentVersionManager) MSIPackage() *windowsagent.Package {
	return avm.msiPackage
}
