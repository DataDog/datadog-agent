// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"

// Params contains the optional parameters for the Datadog Install Script command
type Params struct {
	installerURL string
	// For now the extraEnvVars are only used by the install script,
	// but they can (and should) be passed to the executable.
	extraEnvVars map[string]string
}

// Option is an optional function parameter type for the Params
type Option func(*Params) error

// WithExtraEnvVars specifies additional environment variables.
func WithExtraEnvVars(envVars map[string]string) Option {
	return func(params *Params) error {
		params.extraEnvVars = envVars
		return nil
	}
}

// WithInstallerURL uses a specific URL for the Datadog Installer Install command instead of using the pipeline URL.
func WithInstallerURL(installerURL string) Option {
	return func(params *Params) error {
		params.installerURL = installerURL
		return nil
	}
}

// WithInstallerURLFromInstallersJSON uses a specific URL for the Datadog Installer from an installers_v2.json
// file.
// jsonURL: The URL of the installers_v2.json file, i.e. pipeline.StableURL
// version: The artifact version to retrieve, i.e. "7.56.0-installer-0.4.5-1"
//
// Example: WithInstallerURLFromInstallersJSON(pipeline.StableURL, "7.56.0-installer-0.4.5-1")
// will look into "https://s3.amazonaws.com/ddagent-windows-stable/stable/installers_v2.json" for the Datadog Installer
// version "7.56.0-installer-0.4.5-1"
func WithInstallerURLFromInstallersJSON(jsonURL, version string) Option {
	return func(params *Params) error {
		url, err := installers.GetProductURL(jsonURL, "datadog-installer", version, "x86_64")
		if err != nil {
			return err
		}
		params.installerURL = url
		return nil
	}
}

// MsiParams contains the optional parameters for the Datadog Installer Install command
type MsiParams struct {
	Params
	msiArgs        []string
	msiLogFilename string
}

// MsiOption is an optional function parameter type for the Datadog Installer Install command
type MsiOption func(*MsiParams) error

// WithOption converts an Option to an MsiOption (downcast)
// allowing to use the base Option methods on with a func that accepts MsiOptions
func WithOption(opt Option) MsiOption {
	return func(params *MsiParams) error {
		return opt(&params.Params)
	}
}

// WithMSIArg uses a specific URL for the Datadog Installer Install command instead of using the pipeline URL.
func WithMSIArg(arg string) MsiOption {
	return func(params *MsiParams) error {
		params.msiArgs = append(params.msiArgs, arg)
		return nil
	}
}

// WithMSILogFile sets the filename for the MSI log file, to be stored in the output directory.
func WithMSILogFile(filename string) MsiOption {
	return func(params *MsiParams) error {
		params.msiLogFilename = filename
		return nil
	}
}
