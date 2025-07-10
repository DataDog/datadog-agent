// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package install create function to install the agent
package install

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

// ExecutorWithRetry represents a type that can execute a command and return its output
type ExecutorWithRetry interface {
	ExecuteWithRetry(command string) (output string, err error)
}

// Unix install the agent from install script, by default will install the agent 7 build corresponding to the CI if running in the CI, else the latest Agent 7 version
func Unix(t *testing.T, client ExecutorWithRetry, options ...installparams.Option) {
	params := installparams.NewParams(options...)
	commandLine := ""

	if params.PipelineID != "" && params.MajorVersion != "5" {
		testEnvVars := []string{}
		testEnvVars = append(testEnvVars, fmt.Sprintf("TESTING_APT_URL=s3.amazonaws.com/apttesting.datad0g.com/datadog-agent/pipeline-%v-a%v", params.PipelineID, params.MajorVersion))
		// apt testing repo
		// TESTING_APT_REPO_VERSION="pipeline-xxxxx-ay y"
		testEnvVars = append(testEnvVars, fmt.Sprintf(`TESTING_APT_REPO_VERSION="stable-%v %v"`, params.Arch, params.MajorVersion))
		testEnvVars = append(testEnvVars, "TESTING_YUM_URL=s3.amazonaws.com/yumtesting.datad0g.com")
		// yum testing repo
		// TESTING_YUM_VERSION_PATH="testing/pipeline-xxxxx-ay/y"
		testEnvVars = append(testEnvVars, fmt.Sprintf(`TESTING_YUM_VERSION_PATH="testing/pipeline-%v-a%v/%v"`, params.PipelineID, params.MajorVersion, params.MajorVersion))
		commandLine = strings.Join(testEnvVars, " ")
	} else {
		commandLine = fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", params.MajorVersion)
	}

	if params.Flavor != "" {
		commandLine += fmt.Sprintf(" DD_AGENT_FLAVOR=%s ", params.Flavor)
	}

	if params.Upgrade {
		commandLine += "DD_UPGRADE=true "
	}

	var apikey string
	if params.APIKey != "" {
		apikey = params.APIKey
	} else {
		apikey = "aaaaaaaaaa"

		// If the API key is not provided, disable the telemetry to avoid 403 errors
		commandLine += " DD_INSTRUMENTATION_TELEMETRY_ENABLED=false "
	}

	t.Run("Installing the agent", func(tt *testing.T) {
		var downloadCmd string
		var source string
		if params.MajorVersion != "5" {
			source = "S3"
			downloadCmd = fmt.Sprintf(`curl -L  https://install.datadoghq.com/scripts/install_script_agent%v.sh > installscript.sh`, params.MajorVersion)
		} else {
			source = "dd-agent repository"
			downloadCmd = "curl -L https://raw.githubusercontent.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh > installscript.sh"
		}

		_, err := client.ExecuteWithRetry(downloadCmd)
		require.NoError(tt, err, "failed to download install script from %s: ", source, err)

		cmd := fmt.Sprintf(`DD_API_KEY="%s" %v DD_SITE="datadoghq.eu" bash installscript.sh`, apikey, commandLine)
		output, err := client.ExecuteWithRetry(cmd)
		tt.Log(output)
		require.NoError(tt, err, "agent installation should not return any error: ", err)
	})
}
