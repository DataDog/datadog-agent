// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package install create function to install the agent
package install

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/agent-platform/common"
	"github.com/stretchr/testify/require"
)

// Unix install the agent from install script
func Unix(t *testing.T, client *common.ExtendedClient) {
	t.Run("Installing the agent", func(tt *testing.T) {
		cmd := `DD_API_KEY="aaaaaaaaaa" DD_SITE="datadoghq.eu" bash -c "$(curl -L https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh)"`
		_, err := client.VMClient.ExecuteWithError(cmd)
		require.NoError(tt, err, "agent installation should not return any error: ", err)
	})
}
