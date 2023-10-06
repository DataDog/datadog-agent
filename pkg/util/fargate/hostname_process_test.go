// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build fargateprocess

package fargate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFargateHost(t *testing.T) {
	assert := assert.New(t)
	for _, tc := range []struct {
		orch     OrchestratorName
		ecsFunc  func(context.Context) (string, error)
		eksFunc  func(context.Context) (string, error)
		expected string
		wantErr  bool
	}{
		{
			ECS,
			func(ctx context.Context) (string, error) { return "fargate_task:arn-xxx", nil },
			func(ctx context.Context) (string, error) { return "fargate-ip-xxx", nil },
			"fargate_task:arn-xxx",
			false,
		},
		{
			EKS,
			func(ctx context.Context) (string, error) { return "fargate_task:arn-xxx", nil },
			func(ctx context.Context) (string, error) { return "fargate-ip-xxx", nil },
			"fargate-ip-xxx",
			false,
		},
		{
			Unknown,
			func(ctx context.Context) (string, error) { return "fargate_task:arn-xxx", nil },
			func(ctx context.Context) (string, error) { return "fargate-ip-xxx", nil },
			"",
			true,
		},
	} {
		got, err := getFargateHost(context.TODO(), tc.orch, tc.ecsFunc, tc.eksFunc)
		assert.Equal(err != nil, tc.wantErr)
		assert.Equal(tc.expected, got)
	}
}
