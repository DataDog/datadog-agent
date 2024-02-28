// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
)

// InstallAgentParams are the parameters used for installing the Agent using msiexec.
type InstallAgentParams struct {
	AgentUser         string `installer_arg:"DDAGENTUSER_NAME"`
	AgentUserPassword string `installer_arg:"DDAGENTUSER_PASSWORD"`
	Site              string `installer_arg:"SITE"`
	DdURL             string `installer_arg:"DD_URL"`
	APIKey            string `installer_arg:"APIKEY"`
	InstallLogFile    string
	Package           *Package
}

// InstallAgentOption is an optional function parameter type for InstallAgentParams options
type InstallAgentOption = func(*InstallAgentParams) error

// WithAgentUser specifies the DDAGENTUSER_NAME parameter.
func WithAgentUser(username string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.AgentUser = username
		return nil
	}
}

// WithAgentUserPassword specifies the DDAGENTUSER_PASSWORD parameter.
func WithAgentUserPassword(password string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.AgentUserPassword = password
		return nil
	}
}

// WithSite specifies the SITE parameter.
func WithSite(site string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Site = site
		return nil
	}
}

// WithDdURL specifies the DD_URL parameter.
func WithDdURL(ddURL string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.DdURL = ddURL
		return nil
	}
}

// WithAPIKey specifies the APIKEY parameter.
func WithAPIKey(apiKey string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.APIKey = apiKey
		return nil
	}
}

// WithValidAPIKey sets a valid API key fetched from the runner secret store.
func WithValidAPIKey() InstallAgentOption {
	return func(i *InstallAgentParams) error {
		apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if err != nil {
			return err
		}
		i.APIKey = apiKey
		return nil
	}
}

// WithInstallLogFile specifies the file where to save the MSI install logs.
func WithInstallLogFile(logFileName string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.InstallLogFile = logFileName
		return nil
	}
}

// WithPackage specifies the Agent installation package.
func WithPackage(agentPackage *Package) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Package = agentPackage
		return nil
	}
}

// WithLastStablePackage specifies to use the last stable installation package.
func WithLastStablePackage() InstallAgentOption {
	return func(i *InstallAgentParams) error {
		lastStablePackage, err := GetLastStablePackageFromEnv()
		if err != nil {
			return err
		}
		i.Package = lastStablePackage
		return nil
	}
}

// WithFakeIntake configures the Agent to use a fake intake URL.
func WithFakeIntake(fakeIntake *components.FakeIntake) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.DdURL = fakeIntake.URL
		return nil
	}
}
