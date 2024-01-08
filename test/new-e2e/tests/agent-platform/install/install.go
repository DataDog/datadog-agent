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

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
	"github.com/stretchr/testify/require"
)

// Unix install the agent from install script, by default will install the agent 7 build corresponding to the CI if running in the CI, else the latest Agent 7 version
func Unix(t *testing.T, client *common.TestClient, options ...installparams.Option) {
	params := installparams.NewParams(options...)
	commandLine := ""

	if params.PipelineID != "" && params.MajorVersion != "5" {
		testEnvVars := []string{}
		testEnvVars = append(testEnvVars, "TESTING_APT_URL=apttesting.datad0g.com")
		// apt testing repo
		// TESTING_APT_REPO_VERSION="pipeline-xxxxx-ay y"
		testEnvVars = append(testEnvVars, fmt.Sprintf(`TESTING_APT_REPO_VERSION="pipeline-%v-a%v-%s %v"`, params.PipelineID, params.MajorVersion, params.Arch, params.MajorVersion))
		testEnvVars = append(testEnvVars, "TESTING_YUM_URL=yumtesting.datad0g.com")
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
	}

	t.Run("Installing the agent", func(tt *testing.T) {
		var downdloadCmd string
		var source string
		if params.MajorVersion != "5" {
			source = "S3"
			downdloadCmd = fmt.Sprintf(`curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent%v.sh > installscript.sh`, params.MajorVersion)
		} else {
			source = "dd-agent repository"
			downdloadCmd = "curl -L https://raw.githubusercontent.com/DataDog/dd-agent/master/packaging/datadog-agent/source/install_agent.sh > installscript.sh"
		}

		_, err := client.ExecuteWithRetry(downdloadCmd)
		require.NoError(tt, err, "failed to download install script from %s: ", source, err)

		cmd := fmt.Sprintf(`DD_API_KEY="%s" %v DD_SITE="datadoghq.eu" bash installscript.sh`, apikey, commandLine)
		output, err := client.Host.Execute(cmd)
		tt.Log(output)
		require.NoError(tt, err, "agent installation should not return any error: ", err)
	})
}
