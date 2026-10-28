// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
)

func (suite *clusterAgentSuite) TestGetNodeStatus() {
	dca, err := newDummyClusterAgent(suite.config)
	require.NoError(suite.T(), err)
	dca.rawResponses["/api/v1/info/node/node1/status"] = `{"conditions":[{"type":"Maintenance","status":"True"}],"unschedulable":true}`
	dca.rawResponses["/api/v1/info/node/node2/status"] = `{"unschedulable":false}`

	ts, p, err := dca.StartTLS()
	require.NoError(suite.T(), err)
	defer ts.Close()
	suite.config.SetInTest("cluster_agent.url", fmt.Sprintf("https://127.0.0.1:%d", p))
	ipcmock.New(suite.T())

	ca, err := GetClusterAgentClient()
	require.NoError(suite.T(), err)

	node, err := ca.GetNodeStatus("node1")
	require.NoError(suite.T(), err)
	require.NotNil(suite.T(), node.Conditions)
	assert.Equal(suite.T(), []NodeCondition{{Type: "Maintenance", Status: "True"}}, *node.Conditions)
	assert.True(suite.T(), node.Unschedulable)

	// An answer without conditions is told apart from a node without conditions.
	node, err = ca.GetNodeStatus("node2")
	require.NoError(suite.T(), err)
	assert.Nil(suite.T(), node.Conditions)

	_, err = ca.GetNodeStatus("unknown")
	assert.Error(suite.T(), err)
}
