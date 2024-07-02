// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package command

const (
	// DatadogCodeSignatureThumbprint is the thumbprint of the Datadog Code Signing certificate
	// Valid From: May 2023
	// Valid To:   May 2025
	DatadogCodeSignatureThumbprint = `B03F29CC07566505A718583E9270A6EE17678742`
	// RegistryKeyPath is the root registry key that the Datadog Agent uses to store some state
	RegistryKeyPath = "HKLM:\\SOFTWARE\\Datadog\\Datadog Agent"
	// DefaultInstallPath is the default install path for the Datadog Agent
	DefaultInstallPath = `C:\Program Files\Datadog\Datadog Agent`
	// DefaultConfigRoot is the default config root for the Datadog Agent
	DefaultConfigRoot = `C:\ProgramData\Datadog`
	// DefaultAgentUserName is the default user name for the Datadog Agent
	DefaultAgentUserName = `ddagentuser`
)

// GetDatadogAgentProductCode returns the product code GUID for the Datadog Agent
func GetDatadogAgentProductCode() string {
	return GetProductCodeByName("Datadog Agent")
}

// GetInstallPathFromRegistry gets the install path from the registry, e.g. C:\Program Files\Datadog\Datadog Agent
func GetInstallPathFromRegistry() string {
	return GetRegistryValue(RegistryKeyPath, "InstallPath")
}

// GetConfigRootFromRegistry gets the config root from the registry, e.g. C:\ProgramData\Datadog
func GetConfigRootFromRegistry() string {
	return GetRegistryValue(RegistryKeyPath, "ConfigRoot")
}
