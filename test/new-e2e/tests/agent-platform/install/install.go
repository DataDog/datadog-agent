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
	if params.PipelineID != "" {
		testEnvVars := []string{}
		testEnvVars = append(testEnvVars, "TESTING_APT_URL=apttesting.datad0g.com")
		// apt testing repo
		// TESTING_APT_REPO_VERSION="pipeline-xxxxx-ay y"
		testEnvVars = append(testEnvVars, fmt.Sprintf(`TESTING_APT_REPO_VERSION="pipeline-%v-a%v %v"`, params.PipelineID, params.MajorVersion, params.MajorVersion))
		testEnvVars = append(testEnvVars, "TESTING_YUM_URL=yumtesting.datad0g.com")
		// yum testing repo
		// TESTING_YUM_VERSION_PATH="testing/pipeline-xxxxx-ay/y"
		testEnvVars = append(testEnvVars, fmt.Sprintf("TESTING_YUM_VERSION_PATH=testing/pipeline-%v-a%v/%v", params.PipelineID, params.MajorVersion, params.MajorVersion))
		commandLine = strings.Join(testEnvVars, " ")
	} else {
		commandLine = fmt.Sprintf("DD_AGENT_MAJOR_VERSION=%s", params.MajorVersion)
	}

	t.Run("Installing the agent", func(tt *testing.T) {
		cmd := fmt.Sprintf(`DD_API_KEY="aaaaaaaaaa" %v DD_SITE="datadoghq.eu" bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"`, commandLine)
		output, err := client.VMClient.ExecuteWithError(cmd)
		tt.Log(output)
		require.NoError(tt, err, "agent installation should not return any error: ", err)
	})
}
