// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

// Package ecsfargate implements the ECS Fargate Workloadmeta collector.
package ecsfargate

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
	v2 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2"
)

func TestPullWithTaskCollectionEnabledWithV2Parser(t *testing.T) {
	// Start a dummy Http server to simulate ECS Fargate metadata v2 endpoint
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v2/metadata", "./testdata/redis.json"),
	)
	require.Nil(t, err)
	ts := dummyECS.Start()
	defer ts.Close()

	store := &fakeWorkloadmetaStore{}
	// create an ECS Fargate collector with orchestratorECSCollectionEnabled enabled
	collector := collector{
		store:                 store,
		taskCollectionEnabled: true,
		metaV2:                v2.NewClient(fmt.Sprintf("%s/v2/metadata", ts.URL)),
	}
	collector.taskCollectionParser = collector.parseTaskFromV2Endpoint

	err = collector.Pull(context.Background())
	require.Nil(t, err)
	// one ECS task event and three container events should be notified
	require.Len(t, store.notifiedEvents, 4)

	count := 0
	for _, event := range store.notifiedEvents {
		require.Equal(t, workloadmeta.EventTypeSet, event.Type)
		require.Equal(t, workloadmeta.SourceRuntime, event.Source)
		switch entity := event.Entity.(type) {
		case *workloadmeta.ECSTask:
			require.Equal(t, "us-east-1", entity.Region)
			require.Equal(t, 123457279990, entity.AWSAccountID)
			require.Equal(t, "ecs-cluster", entity.ClusterName)
			require.Equal(t, "my-redis", entity.Family)
			require.Equal(t, "1", entity.Version)
			require.Equal(t, workloadmeta.ECSLaunchTypeFargate, entity.LaunchType)
			require.ElementsMatch(t, entity.Containers, []workloadmeta.OrchestratorContainer{
				{
					ID:   "938f6d263c464aa5985dc67ab7f38a7e-1714341083",
					Name: "log_router",
				},
				{
					ID:   "938f6d263c464aa5985dc67ab7f38a7e-2537586469",
					Name: "datadog-agent",
				},
				{
					ID:   "938f6d263c464aa5985dc67ab7f38a7e-3054012820",
					Name: "redis",
				},
			})
			count++
		case *workloadmeta.Container:
			require.Equal(t, workloadmeta.ContainerRuntimeECSFargate, entity.Runtime)
			if entity.Image.Name == "public.ecr.aws/datadog/agent" {
				require.Equal(t, "datadog-agent", entity.Name)
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 3)
				require.Equal(t, map[string]string{"awsvpc": "172.31.115.123"}, entity.NetworkIPs)
				ts, err := time.Parse(time.RFC3339Nano, "2023-11-20T12:10:44.404563253Z")
				require.NoError(t, err)
				require.Equal(t, workloadmeta.ContainerState{
					Running:   true,
					Status:    workloadmeta.ContainerStatusRunning,
					StartedAt: ts,
					CreatedAt: ts,
				}, entity.State)
				count++
			} else if entity.Image.Name == "redis/redis" {
				require.Equal(t, "redis", entity.Name)
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 3)
				require.Equal(t, map[string]string{"awsvpc": "172.31.115.18"}, entity.NetworkIPs)
				ts, err := time.Parse(time.RFC3339Nano, "2023-11-20T12:11:16.701115523Z")
				require.NoError(t, err)
				require.Equal(t, workloadmeta.ContainerState{
					Running:   true,
					Status:    workloadmeta.ContainerStatusRunning,
					StartedAt: ts,
					CreatedAt: ts,
				}, entity.State)
				count++
			} else if entity.Image.Name == "amazon/aws-for-fluent-bit" {
				require.Equal(t, "log_router", entity.Name)
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 4)
				require.Equal(t, map[string]string{"awsvpc": "172.31.15.128"}, entity.NetworkIPs)
				ts, err := time.Parse(time.RFC3339Nano, "2023-11-20T12:10:44.559880428Z")
				require.NoError(t, err)
				require.Equal(t, workloadmeta.ContainerState{
					Running:   true,
					Status:    workloadmeta.ContainerStatusRunning,
					StartedAt: ts,
					CreatedAt: ts,
				}, entity.State)
				count++
			} else {
				t.Errorf("unexpected image name: %s", entity.Image.Name)
			}
		default:
			t.Errorf("unexpected entity type: %T", entity)
		}
	}
	require.Equal(t, 4, count)
}
