// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

// import (
// 	"context"
// 	"testing"

// 	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
// 	"github.com/pulumi/pulumi/sdk/v3/go/auto"
// 	"github.com/stretchr/testify/require"
// 	"github.com/vboulineau/pulumi-definitions/aws/ecs/ecs"
// )

// func TestAgentOnECS(t *testing.T) {
// 	stack, err := infra.NewStack(context.Background(), "vboulineau", "ecs-ci", ecs.Run, auto.ConfigMap{
// 		"PREFIX": auto.ConfigValue{
// 			Value: "vbprefix",
// 		},
// 		"ENVIRONMENT": auto.ConfigValue{
// 			Value: "sandbox",
// 		},
// 		"aws:region": auto.ConfigValue{
// 			Value: "us-east-1",
// 		},
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, stack)

// 	require.NoError(t, stack.Up(context.Background()))

// 	// Agent tests using ECS Infra goes here

// 	require.NoError(t, stack.Down(context.Background()))
// }
