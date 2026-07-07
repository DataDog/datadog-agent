// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agent

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

func TestLinuxInstallCommand(t *testing.T) {
	t.Run("version-based install inlines the api key", func(t *testing.T) {
		version := agentparams.PackageVersion{Major: "7", Channel: agentparams.StableChannel}
		got := LinuxInstallCommand(os.AMD64Arch, version, "abc123", nil)

		want := `for i in 1 2 3 4 5; do curl -fsSL https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh -o install-script.sh && break || sleep $((2**$i)); done &&  for i in 1 2 3; do DD_API_KEY=abc123 DD_AGENT_MAJOR_VERSION=7 DD_INSTALL_ONLY=true bash install-script.sh  && exit 0 || sleep $((2**$i)); done; exit 1`
		assert.Equal(t, want, got)
	})

	t.Run("pipeline-based install uses the testing repos", func(t *testing.T) {
		version := agentparams.PackageVersion{Major: "7", PipelineID: "12345", Flavor: "datadog-agent"}
		got := LinuxInstallCommand(os.AMD64Arch, version, "abc123", nil)

		assert.Contains(t, got, "DD_API_KEY=abc123 ")
		assert.Contains(t, got, "TESTING_APT_URL=apttesting.datad0g.com/datadog-agent/pipeline-12345-a7")
		assert.Contains(t, got, `TESTING_APT_REPO_VERSION="stable-x86_64 7"`)
		assert.Contains(t, got, "TESTING_YUM_VERSION_PATH=testing/pipeline-12345-a7/7")
		assert.Contains(t, got, "DD_AGENT_FLAVOR=datadog-agent")
		assert.Contains(t, got, "install_script_agent7.sh")
	})

	t.Run("template carries exactly one format verb (the api key)", func(t *testing.T) {
		// The command must contain no stray `%` verbs, otherwise inlining the key would corrupt it.
		version := agentparams.PackageVersion{Major: "7", PipelineID: "12345", Flavor: "datadog-agent"}
		template := linuxInstallCommandTemplate(os.AMD64Arch, version, nil)
		require.Equal(t, 1, strings.Count(template, "%"), "template must contain exactly one %% verb")
		assert.Equal(t, fmt.Sprintf(template, "abc123"), LinuxInstallCommand(os.AMD64Arch, version, "abc123", nil))
	})
}
