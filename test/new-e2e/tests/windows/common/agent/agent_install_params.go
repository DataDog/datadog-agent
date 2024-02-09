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

type InstallAgentParams struct {
	AgentUser         string // DDAGENTUSER_NAME
	AgentUserPassword string // DDAGENTUSER_PASSWORD
	Site              string // SITE
	DdUrl             string // DD_URL
	ApiKey            string // APIKEY
	InstallLogFile    string
	Package           *Package
}
type InstallAgentOption = func(*InstallAgentParams) error

func WithAgentUser(username string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.AgentUser = username
		return nil
	}
}

func WithAgentUserPassword(password string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.AgentUserPassword = password
		return nil
	}
}

func WithSite(site string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Site = site
		return nil
	}
}

func WithDdUrl(ddUrl string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.DdUrl = ddUrl
		return nil
	}
}

func WithApiKey(apiKey string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ApiKey = apiKey
		return nil
	}
}

func WithInstallLogFile(logFileName string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.InstallLogFile = logFileName
		return nil
	}
}

func WithValidApiKey() InstallAgentOption {
	return func(i *InstallAgentParams) error {
		apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
		if err != nil {
			return err
		}
		i.ApiKey = apiKey
		return nil
	}
}

func WithPackage(agentPackage *Package) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Package = agentPackage
		return nil
	}
}

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

func WithFakeIntake(fakeIntake *components.FakeIntake) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.DdUrl = fakeIntake.URL
		return nil
	}
}
