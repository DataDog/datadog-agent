// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dummyEndpointsConfigs = `{
"last_change": 42,
"configs": [
  {
    "check_name": "one"
  },
  {
    "check_name": "two"
  }
]
}`

func (suite *clusterAgentSuite) TestEndpointsChecksNominal() {
	ctx := context.Background()
	dca, err := newDummyClusterAgent()
	require.NoError(suite.T(), err)

	dca.rawResponses["/api/v1/endpointschecks/configs/mynode"] = dummyEndpointsConfigs

	ts, p, err := dca.StartTLS()
	require.NoError(suite.T(), err)
	defer ts.Close()
	mockConfig.SetWithoutSource("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.NoError(suite.T(), err)

	configs, err := ca.GetEndpointsCheckConfigs(ctx, "mynode")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(42), configs.LastChange)
	require.Len(suite.T(), configs.Configs, 2)
	assert.Equal(suite.T(), "one", configs.Configs[0].Name)
	assert.Equal(suite.T(), "two", configs.Configs[1].Name)
}
