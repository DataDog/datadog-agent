// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build fargateprocess

package fargate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFargateHost(t *testing.T) {
	assert := assert.New(t)
	for _, tc := range []struct {
		orch     OrchestratorName
		ecsFunc  func() (string, error)
		eksFunc  func() (string, error)
		expected string
		wantErr  bool
	}{
		{
			ECS,
			func() (string, error) { return "fargate_task:arn-xxx", nil },
			func() (string, error) { return "fargate-ip-xxx", nil },
			"fargate_task:arn-xxx",
			false,
		},
		{
			EKS,
			func() (string, error) { return "fargate_task:arn-xxx", nil },
			func() (string, error) { return "fargate-ip-xxx", nil },
			"fargate-ip-xxx",
			false,
		},
		{
			Unknown,
			func() (string, error) { return "fargate_task:arn-xxx", nil },
			func() (string, error) { return "fargate-ip-xxx", nil },
			"",
			true,
		},
	} {
		got, err := getFargateHost(tc.orch, tc.ecsFunc, tc.eksFunc)
		assert.Equal(err != nil, tc.wantErr)
		assert.Equal(tc.expected, got)
	}
}
