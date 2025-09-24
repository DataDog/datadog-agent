// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package consts list the common packages paths used in the Datadog Installer tests.
package consts

import (
	"fmt"
	"path"
)

const (
	// AgentPackage is the name of the Datadog Agent package
	// We use a constant to make it easier for calling code, because depending on the context
	// the Agent package can be referred to as "agent-package" (like in the OCI registry) or "datadog-agent" (in the
	// local database once the Agent is installed).
	AgentPackage string = "datadog-agent"
	// InstallerPackage is the name of the Datadog Installer package
	InstallerPackage string = "datadog-installer"
	// Path is the path where the Datadog Installer is installed on disk
	Path string = "C:\\Program Files\\Datadog\\Datadog Agent\\bin"
	// BinaryName is the name of the Datadog Installer binary on disk
	BinaryName string = "datadog-installer.exe"
	// ServiceName the installer service name
	ServiceName string = "Datadog Installer"
	// ConfigPath is the location of the Datadog Installer's configuration on disk
	ConfigPath string = "C:\\ProgramData\\Datadog\\datadog.yaml"
	// RegistryKeyPath is the root registry key that the Datadog Installer uses to store some state
	RegistryKeyPath string = `HKLM:\SOFTWARE\Datadog\Datadog Agent`
	// NamedPipe is the name of the named pipe used by the Datadog Installer
	NamedPipe string = `\\.\pipe\dd_installer`

	// baseConfigPath is the base path for the Installer configuration
	baseConfigPath = "C:/ProgramData/Datadog/Installer"

	// PipelineOCIRegistry is the OCI registry that pipelines submit packages to
	// Use special domain instead of cloudfront to avoid NAT gateway costs
	// Can't use s3 domain directly because bucket name contains a dot
	PipelineOCIRegistry = "installtesting.datad0g.com.internal.dda-testing.com"

	// BetaS3OCIRegistry is the OCI registry that rc/beta packages are submitted to
	// Use special domain instead of cloudfront to avoid NAT gateway costs
	// Can't use s3 domain directly because bucket name contains a dot
	BetaS3OCIRegistry = "install.datad0g.com.internal.dda-testing.com"

	// StableS3OCIRegistry is the OCI registry that stable packages are submitted to
	// Use s3 domain instead of cloudfront to avoid NAT gateway costs
	StableS3OCIRegistry = "dd-agent.s3.amazonaws.com"
)

var (
	// BinaryPath is the path of the Datadog Installer binary on disk
	BinaryPath = path.Join(Path, BinaryName)

	// InstallerConfigPaths are the paths that the Datadog Installer uses to store its working files.
	// They are normally created by the install script / bootstrap.
	InstallerConfigPaths = []string{
		path.Join(baseConfigPath, "packages"),
		path.Join(baseConfigPath, "configs"),
		path.Join(baseConfigPath, "tmp"),
	}
)

// GetExperimentDirFor is the path to the experiment symbolic link on disk
func GetExperimentDirFor(packageName string) string {
	return fmt.Sprintf("%s\\packages\\%s\\experiment", baseConfigPath, packageName)
}

// GetStableDirFor is the path to the stable symbolic link on disk
func GetStableDirFor(packageName string) string {
	return fmt.Sprintf("%s\\packages\\%s\\stable", baseConfigPath, packageName)
}
