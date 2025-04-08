// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"

	agentVersion "github.com/DataDog/datadog-agent/pkg/version"
	windowsagent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

// AgentVersionManager provides Agent package information for a particular Agent version for the installer tests
type AgentVersionManager struct {
	// version should match Agent's `version` subcommand output field
	// Example Agent: Agent 7.65.0-devel - Meta: git.579.0000ac2 - Commit: 0000ac28cd - Serialization version: v5.0.144 - Go version: go1.23.7
	// Example Installer: 7.65.0-devel+git.579.0000ac2
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
		return nil, fmt.Errorf("failed to parse version %s: %w", versionStr, err)
	}
	// sanity check format of packageVersionStr
	// NOTE: we don't use the struct result, GetNumberAndPre() does not work on
	//       url-safe package strings because its regex expects a `+` character,
	//       which is not present in the url-safe package version string.
	_, err = agentVersion.New(packageVersionStr, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse package version %s: %w", packageVersionStr, err)
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
// this should match Agent's `version` subcommand output field
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
