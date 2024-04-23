// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/testutil"
	v1 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v1"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

// TestPullWithTaskCollectionEnabledWithV4Parser tests the Pull method with taskCollectionEnabled enabled
// and taskCollectionParser set to parseTasksFromV4Endpoint to parse the tasks from the v4 metadata endpoint
func TestPullWithTaskCollectionEnabledWithV4Parser(t *testing.T) {
	// Start a dummy Http server to simulate ECS metadata endpoints
	// /v1/tasks: return the list of tasks containing datadog-agent task and nginx task
	ts, err := getDummyECS()
	require.NoError(t, err)
	defer ts.Close()

	// Add container handler to return the v4 endpoints for different containers
	store := getFakeWorkloadmetaStore(ts.URL)

	// create an ECS collector with v4TaskEnabled enabled
	collector := collector{
		store:                 store,
		taskCollectionEnabled: true,
		metaV1:                v1.NewClient(ts.URL),
		metaV3or4: func(metaURI, metaVersion string) v3or4.Client {
			return v3or4.NewClient(metaURI, metaVersion)
		},
		taskCache:     cache.New(3*time.Minute, 30*time.Second),
		taskRateRPS:   35,
		taskRateBurst: 60,
	}

	collector.taskCollectionParser = collector.parseTasksFromV4Endpoint

	err = collector.Pull(context.Background())
	require.NoError(t, err)
	// two ECS task events and two container events should be notified
	require.Len(t, store.notifiedEvents, 4)

	count := 0
	for _, event := range store.notifiedEvents {
		require.Equal(t, workloadmeta.EventTypeSet, event.Type)
		require.Equal(t, workloadmeta.SourceNodeOrchestrator, event.Source)
		switch entity := event.Entity.(type) {
		case *workloadmeta.ECSTask:
			require.Equal(t, 123457279990, entity.AWSAccountID)
			require.Equal(t, "us-east-1", entity.Region)
			require.Equal(t, "ecs-cluster", entity.ClusterName)
			require.Equal(t, "RUNNING", entity.DesiredStatus)
			require.Equal(t, workloadmeta.ECSLaunchTypeEC2, entity.LaunchType)
			if entity.Family == "datadog-agent" {
				require.Equal(t, "15", entity.Version)
				require.Equal(t, "vpc-123", entity.VPCID)
				count++
			} else if entity.Family == "nginx" {
				require.Equal(t, "3", entity.Version)
				require.Equal(t, "vpc-124", entity.VPCID)
				count++
			} else {
				t.Errorf("unexpected entity family: %s", entity.Family)
			}
		case *workloadmeta.Container:
			require.Equal(t, "RUNNING", entity.KnownStatus)
			require.Equal(t, "HEALTHY", entity.Health.Status)
			if entity.Image.Name == "datadog/datadog-agent" {
				require.Equal(t, "7.50.0", entity.Image.Tag)
				require.Equal(t, "Agent health: PASS", entity.Health.Output)
				count++
			} else if entity.Image.Name == "ghcr.io/nginx/my-nginx" {
				require.Equal(t, "ghcr.io", entity.Image.Registry)
				require.Equal(t, "main", entity.Image.Tag)
				require.Equal(t, "Nginx health: PASS", entity.Health.Output)
				count++
			} else {
				t.Errorf("unexpected image name: %s", entity.Image.Name)
			}
		default:
			t.Errorf("unexpected entity type: %T", entity)
		}
	}
	require.Equal(t, 4, count)

	// second pull should not notify any events as they are in cache
	store.notifiedEvents = store.notifiedEvents[:0]
	err = collector.Pull(context.Background())
	require.NoError(t, err)
	require.Len(t, store.notifiedEvents, 0)

}

func getDummyECS() (*httptest.Server, error) {
	dummyECS, err := testutil.NewDummyECS(
		testutil.FileHandlerOption("/v4/1234-1/taskWithTags", "./testdata/datadog-agent.json"),
		testutil.FileHandlerOption("/v4/1234-2/taskWithTags", "./testdata/nginx.json"),
		testutil.FileHandlerOption("/v1/tasks", "./testdata/tasks.json"),
	)
	ts := dummyECS.Start()
	return ts, err
}

func getFakeWorkloadmetaStore(ecsAgentURL string) *fakeWorkloadmetaStore {
	return &fakeWorkloadmetaStore{
		getGetContainerHandler: func(id string) (*workloadmeta.Container, error) {
			// nginx container ID, see ./testdata/nginx.json
			if id == "2ad9e753a0dfbba1c91e0e7bebaaf3a0918d3ef304b7549b1ced5f573bc05645" {
				// add delay to trigger timeout
				return &workloadmeta.Container{
					EnvVars: map[string]string{
						v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/1234-2", ecsAgentURL),
					},
				}, nil
			}
			// datadog-agent container ID, see ./testdata/datadog-agent.json
			if id == "749d28eb7145ff3b6c52b71c59b381c70a884c1615e9f99516f027492679496e" {
				// add delay to trigger timeout
				return &workloadmeta.Container{
					EnvVars: map[string]string{
						v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/1234-1", ecsAgentURL),
					},
				}, nil
			}
			return &workloadmeta.Container{
				EnvVars: map[string]string{
					v3or4.DefaultMetadataURIv4EnvVariable: fmt.Sprintf("%s/v4/undefined", ecsAgentURL),
				},
			}, nil
		},
	}
}
