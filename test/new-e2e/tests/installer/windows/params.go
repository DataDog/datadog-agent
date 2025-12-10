// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent/installers/v2"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/pipeline"
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

// WithURLFromPipeline uses the Datadog Installer MSI from a pipeline artifact.
func WithURLFromPipeline(pipelineID string) Option {
	return func(params *Params) error {
		artifactURL, err := pipeline.GetPipelineArtifact(pipelineID, pipeline.AgentS3BucketTesting, pipeline.DefaultMajorVersion, func(artifact string) bool {
			return strings.Contains(artifact, "datadog-agent") && strings.HasSuffix(artifact, ".msi")
		})
		if err != nil {
			return err
		}
		params.installerURL = artifactURL
		return nil
	}
}

// WithURLFromInstallersJSON uses a specific URL for the Datadog Installer from an installers_v2.json
// file.
// jsonURL: The URL of the installers_v2.json file, i.e. pipeline.StableURL
// version: The artifact version to retrieve, i.e. "7.56.0-installer-0.4.5-1"
//
// Example: WithInstallerURLFromInstallersJSON(pipeline.StableURL, "7.56.0-installer-0.4.5-1")
// will look into "https://s3.amazonaws.com/ddagent-windows-stable/stable/installers_v2.json" for the Datadog Installer
// version "7.56.0-installer-0.4.5-1"
func WithURLFromInstallersJSON(jsonURL, version string) Option {
	return func(params *Params) error {
		url, err := installers.GetProductURL(jsonURL, "datadog-agent", version, "x86_64")
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

// WithMSIDevEnvOverrides applies overrides to the MSI source config based on environment variables.
//
// Example: local MSI package file
//
//	export CURRENT_AGENT_MSI_URL="file:///path/to/msi/package.msi"
//
// Example: from a different pipeline
//
//	export CURRENT_AGENT_MSI_PIPELINE="123456"
//
// Example: stable version from installers_v2.json
//
//	export CURRENT_AGENT_MSI_VERSION=7.60.0-1"
//
// Example: custom URL
//
//	export CURRENT_AGENT_MSI_URL="https://s3.amazonaws.com/dd-agent-mstesting/builds/beta/ddagent-cli-7.64.0-rc.9.msi"
func WithMSIDevEnvOverrides(prefix string) MsiOption {
	return func(params *MsiParams) error {
		if url, ok := os.LookupEnv(prefix + "_MSI_URL"); ok {
			err := WithOption(WithInstallerURL(url))(params)
			if err != nil {
				return err
			}
		}
		if pipeline, ok := os.LookupEnv(prefix + "_MSI_PIPELINE"); ok {
			err := WithOption(WithURLFromPipeline(pipeline))(params)
			if err != nil {
				return err
			}
		}
		if version, ok := os.LookupEnv(prefix + "_MSI_VERSION"); ok {
			err := WithOption(WithURLFromInstallersJSON(pipeline.StableURL, version))(params)
			if err != nil {
				return err
			}
		}
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
