// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package clusteragent

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
)

var dummyStatusResponse = `{"isuptodate": true}`

var dummyConfigs = `{
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

func (suite *clusterAgentSuite) TestClusterChecksNominal() {
	dca, err := newDummyClusterAgent()
	require.NoError(suite.T(), err)

	dca.rawResponses["/api/v1/clusterchecks/status/mynode"] = dummyStatusResponse
	dca.rawResponses["/api/v1/clusterchecks/configs/mynode"] = dummyConfigs

	ts, p, err := dca.StartTLS()
	defer ts.Close()
	require.NoError(suite.T(), err)
	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))

	ca, err := GetClusterAgentClient()
	require.NoError(suite.T(), err)

	response, err := ca.PostClusterCheckStatus("mynode", types.NodeStatus{})
	require.NoError(suite.T(), err)
	assert.True(suite.T(), response.IsUpToDate)

	configs, err := ca.GetClusterCheckConfigs("mynode")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(42), configs.LastChange)
	require.Len(suite.T(), configs.Configs, 2)
	assert.Equal(suite.T(), "one", configs.Configs[0].Name)
	assert.Equal(suite.T(), "two", configs.Configs[1].Name)
}

func (suite *clusterAgentSuite) TestClusterChecksRedirect() {
	// Leader starts first
	leader, err := newDummyClusterAgent()
	require.NoError(suite.T(), err)
	leader.rawResponses["/api/v1/clusterchecks/status/mynode"] = `{"isuptodate": true}`
	leader.rawResponses["/api/v1/clusterchecks/configs/mynode"] = dummyConfigs
	ts, p, err := leader.StartTLS()
	defer ts.Close()
	require.NoError(suite.T(), err)

	// Follower redirects to the leader
	follower, err := newDummyClusterAgent()
	require.NoError(suite.T(), err)
	follower.redirectURL = fmt.Sprintf("https://127.0.0.1:%d", p)
	follower.rawResponses["/api/v1/clusterchecks/status/mynode"] = `{"isuptodate": false}`
	follower.rawResponses["/api/v1/clusterchecks/configs/mynode"] = dummyConfigs
	ts, p, err = follower.StartTLS()
	defer ts.Close()
	require.NoError(suite.T(), err)

	// Make sure both DCAs have the same token
	assert.Equal(suite.T(), follower.token, leader.token)

	// Client will start at the follower
	mockConfig.Set("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))
	ca, err := GetClusterAgentClient()
	require.NoError(suite.T(), err)

	// First request will be redirected
	response, err := ca.PostClusterCheckStatus("mynode", types.NodeStatus{})
	require.NoError(suite.T(), err)
	assert.True(suite.T(), response.IsUpToDate)

	assert.NotNil(suite.T(), follower.PopRequest(), "request did no go through follower")
	assert.NotNil(suite.T(), leader.PopRequest(), "request did no reach leader")

	// Subsequent requests will bypass the follower
	configs, err := ca.GetClusterCheckConfigs("mynode")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(42), configs.LastChange)
	require.Len(suite.T(), configs.Configs, 2)
	assert.Equal(suite.T(), "one", configs.Configs[0].Name)
	assert.Equal(suite.T(), "two", configs.Configs[1].Name)

	assert.Nil(suite.T(), follower.PopRequest(), "request reached follower")
	assert.NotNil(suite.T(), leader.PopRequest(), "request did no reach leader")

	// Make leader fail, request will be retried on the main URL,
	// and succeed on the new leader
	leader.Lock()
	delete(leader.rawResponses, "/api/v1/clusterchecks/status/mynode")
	leader.Unlock()
	follower.Lock()
	follower.redirectURL = ""
	follower.Unlock()

	response, err = ca.PostClusterCheckStatus("mynode", types.NodeStatus{})
	require.NoError(suite.T(), err, "request should not fail")
	assert.False(suite.T(), response.IsUpToDate)
	assert.NotNil(suite.T(), leader.PopRequest(), "request did no reach leader")
	assert.NotNil(suite.T(), follower.PopRequest(), "request did not reach follower")
}
