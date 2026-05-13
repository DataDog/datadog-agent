// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
)

// Params contains the optional parameters for the Datadog Install Script command
type Params struct {
	installerURL string
	agentUser    string
	// For now the extraEnvVars are only used by the install script,
	// but they can (and should) be passed to the executable.
	installerScript string
	extraEnvVars    map[string]string
	pipelineID      string
}

// Option is an optional function parameter type for the Params
type Option func(*Params) error

// WithAgentUser sets the user to install the agent as
func WithAgentUser(user string) Option {
	return func(params *Params) error {
		params.agentUser = user
		return nil
	}
}

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

// WithInstallerScript uses a specific URL for the Datadog Installer script command instead of using the pipeline script.
func WithInstallerScript(installerScript string) Option {
	return func(params *Params) error {
		params.installerScript = installerScript
		return nil
	}
}

// WithPipelineID sets the pipeline ID to fetch artifacts/scripts from.
func WithPipelineID(id string) Option {
	return func(params *Params) error {
		params.pipelineID = id
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

// WithInstallScriptDevEnvOverrides applies overrides to use local files for development.
//
// Example: local installer exe
//
//	export CURRENT_AGENT_INSTALLER_URL="file:///path/to/installer.exe"
//
// Example: local install script
//
//	export CURRENT_AGENT_INSTALLER_SCRIPT="file:///path/to/install.ps1"
func WithInstallScriptDevEnvOverrides(prefix string) Option {
	return func(params *Params) error {
		if url, ok := os.LookupEnv(prefix + "_INSTALLER_URL"); ok {
			err := WithInstallerURL(url)(params)
			if err != nil {
				return err
			}
		}
		if script, ok := os.LookupEnv(prefix + "_INSTALLER_SCRIPT"); ok {
			err := WithInstallerScript(script)(params)
			if err != nil {
				return err
			}
		}
		return nil
	}
}
