// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/require"
)

func TestOperatorClientUpdateRequest(t *testing.T) {
	c, err := NewUnverifiedGRPCClient(
		"",
		"",
		func() (string, error) { return "", nil },
		WithOperator(true, "operator-version:9.9.9"),
		WithProducts(state.ProductOrchestratorK8sCRDs),
		WithPollInterval(time.Hour),
	)
	require.NoError(t, err)

	r, err := c.newUpdateRequest()
	require.NoError(t, err)
	require.NotNil(t, r)
	require.True(t, r.Client.IsOperator)
	require.False(t, r.Client.IsUpdater || r.Client.IsAgent || r.Client.IsTracer)
	o := r.Client.ClientOperator
	require.NotNil(t, o)
	require.True(t, o.HasAgentRbacs)
	require.Contains(t, o.Tags, "operator-version:9.9.9")
}
