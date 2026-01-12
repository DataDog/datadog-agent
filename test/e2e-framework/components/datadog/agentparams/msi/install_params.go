// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package msi

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
)

// InstallAgentParams are the parameters used for installing the Agent using msiexec.
type InstallAgentParams struct {
	AgentUser         string `installer_arg:"DDAGENTUSER_NAME"`
	AgentUserPassword string `installer_arg:"DDAGENTUSER_PASSWORD"`
	DdURL             string `installer_arg:"DD_URL"`
	Site              string `installer_arg:"SITE"`
	InstallPath       string `installer_arg:"PROJECTLOCATION"`
	InstallLogFile    string `installer_arg:"/log"`
}

// InstallAgentOption is an optional function parameter type for InstallAgentParams options
type InstallAgentOption = func(*InstallAgentParams)

// NewInstallParams instantiates a new InstallAgentParams and runs all the given InstallAgentOption
// Example usage:
//
//	awshost.WithAgentOptions(
//	  agentparams.WithAdditionalInstallParameters(
//		msiparams.NewInstallParams(
//			msiparams.WithAgentUser(fmt.Sprintf("%s\\%s", TestDomain, TestUser)),
//			msiparams.WithAgentUserPassword(TestPassword)))),
func NewInstallParams(msiInstallParams ...InstallAgentOption) []string {
	msiInstallAgentParams := &InstallAgentParams{}
	for _, o := range msiInstallParams {
		o(msiInstallAgentParams)
	}
	return msiInstallAgentParams.ToArgs()
}

// ToArgs convert the params to a list of valid msi switches, based on the `installer_arg` tag
func (p *InstallAgentParams) ToArgs() []string {
	var args []string
	typeOfMSIInstallAgentParams := reflect.TypeOf(*p)
	for fieldIndex := 0; fieldIndex < typeOfMSIInstallAgentParams.NumField(); fieldIndex++ {
		field := typeOfMSIInstallAgentParams.Field(fieldIndex)
		installerArg := field.Tag.Get("installer_arg")
		if installerArg != "" {
			installerArgValue := reflect.ValueOf(*p).FieldByName(field.Name).String()
			if installerArgValue != "" {
				format := "%s=%s"
				if field.Name == "InstallLogFile" {
					format = "%s %s"
				}
				args = append(args, fmt.Sprintf(format, installerArg, installerArgValue))
			}
		}
	}
	return args
}

// WithAgentUser specifies the DDAGENTUSER_NAME parameter.
func WithAgentUser(username string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.AgentUser = username
	}
}

// WithAgentUserPassword specifies the DDAGENTUSER_PASSWORD parameter.
func WithAgentUserPassword(password string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.AgentUserPassword = password
	}
}

// WithSite specifies the SITE parameter.
func WithSite(site string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.Site = site
	}
}

// WithDdURL specifies the DD_URL parameter.
func WithDdURL(ddURL string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.DdURL = ddURL
	}
}

// WithInstallLogFile specifies the file where to save the MSI install logs.
func WithInstallLogFile(logFileName string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.InstallLogFile = logFileName
	}
}

// WithFakeIntake configures the Agent to use a fake intake URL.
func WithFakeIntake(fakeIntake *fakeintake.FakeintakeOutput) InstallAgentOption {
	return WithDdURL(fakeIntake.URL)
}

// WithFakeIntake configures the Agent to use a fake intake URL.
func WithCustomInstallPath(installPath string) InstallAgentOption {
	return func(i *InstallAgentParams) {
		i.InstallPath = installPath
	}
}
