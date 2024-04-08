// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package agent

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams/msi"
)

// InstallAgentParams are the parameters used for installing the Agent using msiexec.
type InstallAgentParams struct {
	Package *Package
	// Path on local test runner to save the MSI install log
	LocalInstallLogFile string

	msi.InstallAgentParams
	// Installer parameters
	WixFailWhenDeferred string `installer_arg:"WIXFAILWHENDEFERRED"`
	// Installer parameters for agent config
	APIKey                  string `installer_arg:"APIKEY"`
	Tags                    string `installer_arg:"TAGS"`
	Hostname                string `installer_arg:"HOSTNAME"`
	CmdPort                 string `installer_arg:"CMD_PORT"`
	ProxyHost               string `installer_arg:"PROXY_HOST"`
	ProxyPort               string `installer_arg:"PROXY_PORT"`
	ProxyUser               string `installer_arg:"PROXY_USER"`
	ProxyPassword           string `installer_arg:"PROXY_PASSWORD"`
	LogsDdURL               string `installer_arg:"LOGS_DD_URL"`
	ProcessDdURL            string `installer_arg:"PROCESS_DD_URL"`
	TraceDdURL              string `installer_arg:"TRACE_DD_URL"`
	LogsEnabled             string `installer_arg:"LOGS_ENABLED"`
	ProcessEnabled          string `installer_arg:"PROCESS_ENABLED"`
	ProcessDiscoveryEnabled string `installer_arg:"PROCESS_DISCOVERY_ENABLED"`
	APMEnabled              string `installer_arg:"APM_ENABLED"`
}

// InstallAgentOption is an optional function parameter type for InstallAgentParams options
type InstallAgentOption = func(*InstallAgentParams) error

func (p *InstallAgentParams) toArgs() []string {
	var args []string
	typeOfInstallAgentParams := reflect.TypeOf(*p)
	for fieldIndex := 0; fieldIndex < typeOfInstallAgentParams.NumField(); fieldIndex++ {
		field := typeOfInstallAgentParams.Field(fieldIndex)
		installerArg := field.Tag.Get("installer_arg")
		if installerArg != "" {
			installerArgValue := reflect.ValueOf(*p).FieldByName(field.Name).String()
			if installerArgValue != "" {
				args = append(args, fmt.Sprintf("%s=%s", installerArg, installerArgValue))
			}
		}
	}
	args = append(args, p.InstallAgentParams.ToArgs()...)

	return args
}

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

// WithInstallLogFile specifies the file on the local test runner to save the MSI install logs.
func WithInstallLogFile(logFileName string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.LocalInstallLogFile = logFileName
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

// WithWixFailWhenDeferred sets the WixFailWhenDeferred parameter.
func WithWixFailWhenDeferred() InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.WixFailWhenDeferred = "1"
		return nil
	}
}

// WithTags specifies the TAGS parameter.
func WithTags(tags string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Tags = tags
		return nil
	}
}

// WithHostname specifies the HOSTNAME parameter.
func WithHostname(hostname string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.Hostname = hostname
		return nil
	}
}

// WithCmdPort specifies the CMD_PORT parameter.
func WithCmdPort(cmdPort string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.CmdPort = cmdPort
		return nil
	}
}

// WithProxyHost specifies the PROXY_HOST parameter.
func WithProxyHost(proxyHost string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProxyHost = proxyHost
		return nil
	}
}

// WithProxyPort specifies the PROXY_PORT parameter.
func WithProxyPort(proxyPort string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProxyPort = proxyPort
		return nil
	}
}

// WithProxyUser specifies the PROXY_USER parameter.
func WithProxyUser(proxyUser string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProxyUser = proxyUser
		return nil
	}
}

// WithProxyPassword specifies the PROXY_PASSWORD parameter.
func WithProxyPassword(proxyPassword string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProxyPassword = proxyPassword
		return nil
	}
}

// WithLogsDdURL specifies the LOGS_DD_URL parameter.
func WithLogsDdURL(logsDdURL string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.LogsDdURL = logsDdURL
		return nil
	}
}

// WithProcessDdURL specifies the PROCESS_DD_URL parameter.
func WithProcessDdURL(processDdURL string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProcessDdURL = processDdURL
		return nil
	}
}

// WithTraceDdURL specifies the TRACE_DD_URL parameter.
func WithTraceDdURL(traceDdURL string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.TraceDdURL = traceDdURL
		return nil
	}
}

// WithLogsEnabled specifies the LOGS_ENABLED parameter.
func WithLogsEnabled(logsEnabled string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.LogsEnabled = logsEnabled
		return nil
	}
}

// WithProcessEnabled specifies the PROCESS_ENABLED parameter, which controls process_collection.
func WithProcessEnabled(processEnabled string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProcessEnabled = processEnabled
		return nil
	}
}

// WithProcessDiscoveryEnabled specifies the PROCESS_DISCOVERY_ENABLED parameter.
func WithProcessDiscoveryEnabled(processDiscoveryEnabled string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.ProcessDiscoveryEnabled = processDiscoveryEnabled
		return nil
	}
}

// WithAPMEnabled specifies the APM_ENABLED parameter.
func WithAPMEnabled(apmEnabled string) InstallAgentOption {
	return func(i *InstallAgentParams) error {
		i.APMEnabled = apmEnabled
		return nil
	}
}
