// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropAppKeySecret(t *testing.T) {
	for name, values := range map[string]HelmValues{
		"linux":   buildLinuxHelmValues("dda", "agent", "7", "cluster-agent", "7", pulumi.String("token"), true, false, false, false),
		"windows": buildWindowsHelmValues("dda", "agent", "7", "cluster-agent", "7"),
	} {
		t.Run(name, func(t *testing.T) {
			datadog, ok := values["datadog"].(pulumi.Map)
			require.True(t, ok)
			require.Contains(t, datadog, "appKeyExistingSecret")

			values.dropAppKeySecret()

			assert.NotContains(t, datadog, "appKeyExistingSecret")
			assert.Contains(t, datadog, "apiKeyExistingSecret", "the API key reference must stay")
		})
	}
}
