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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
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
			require.Equal(t, "ecs-cluster", entity.ClusterName)
			require.Equal(t, "my-redis", entity.Family)
			require.Equal(t, "1", entity.Version)
			require.Equal(t, workloadmeta.ECSLaunchTypeFargate, entity.LaunchType)
			count++
		case *workloadmeta.Container:
			if entity.Image.Name == "public.ecr.aws/datadog/agent" {
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 3)
				count++
			} else if entity.Image.Name == "redis/redis" {
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 3)
				count++
			} else if entity.Image.Name == "amazon/aws-for-fluent-bit" {
				require.Equal(t, "latest", entity.Image.Tag)
				require.Len(t, entity.Labels, 4)
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
