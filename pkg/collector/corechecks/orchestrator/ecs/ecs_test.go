// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package ecs

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/agent-payload/v5/process"

	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/ecs"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	oconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/serializer/types"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestGetRegionAndAWSAccountID(t *testing.T) {
	region, id := getRegionAndAWSAccountID("arn:aws:ecs:us-east-1:123427279990:container-instance/ecs-my-cluster/123412345abcdefgh34999999")
	require.Equal(t, "us-east-1", region)
	require.Equal(t, 123427279990, id)
}
func TestInitClusterID(t *testing.T) {
	id1 := initClusterID(123456789012, "us-east-1", "ecs-cluster-1")
	require.Equal(t, "34616234-6562-3536-3733-656534636532", id1)

	// same account, same region, different cluster name
	id2 := initClusterID(123456789012, "us-east-1", "ecs-cluster-2")
	require.Equal(t, "31643131-3131-3263-3331-383136383336", id2)

	// same account, different region, same cluster name
	id3 := initClusterID(123456789012, "us-east-2", "ecs-cluster-1")
	require.Equal(t, "64663464-6662-3232-3635-646166613230", id3)

	// different account, same region, same cluster name
	id4 := initClusterID(123456789013, "us-east-1", "ecs-cluster-1")
	require.Equal(t, "61623431-6137-6231-3136-366464643761", id4)
}

type fakeWorkloadmetaStore struct {
	workloadmeta.Component
	EnableV4       bool
	notifiedEvents []*workloadmeta.ECSTask
}

func (store *fakeWorkloadmetaStore) AddECSTasks(task ...*workloadmeta.ECSTask) {
	store.notifiedEvents = append(store.notifiedEvents, task...)
}

func (store *fakeWorkloadmetaStore) ListECSTasks() (events []*workloadmeta.ECSTask) {
	return store.notifiedEvents
}

func (store *fakeWorkloadmetaStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if id == "938f6d263c464aa5985dc67ab7f38a7e-1714341083" {
		return container1(store.EnableV4), nil
	}
	if id == "938f6d263c464aa5985dc67ab7f38a7e-1714341084" {
		return container2(store.EnableV4), nil
	}
	return nil, fmt.Errorf("container not found")
}

type fakeSender struct {
	mocksender.MockSender
	messages   []process.MessageBody
	clusterIDs []string
	nodeTypes  []int
}

func (s *fakeSender) OrchestratorMetadata(msgs []types.ProcessMessageBody, clusterID string, nodeType int) {
	s.messages = append(s.messages, msgs...)
	s.clusterIDs = append(s.clusterIDs, clusterID)
	s.nodeTypes = append(s.nodeTypes, nodeType)
}

func (s *fakeSender) Flush() []process.MessageBody {
	messages := s.messages
	s.messages = s.messages[:0]
	return messages
}

func TestNotECS(t *testing.T) {
	check, _, sender := prepareTest(t, false, "notECS")
	err := check.Run()
	require.NoError(t, err)
	require.Len(t, sender.messages, 0)
}

func TestECSV4Enabled(t *testing.T) {
	testECS(true, t)
}

// TestECSV4Disabled tests the ECS collector when the feature of using v4 endpoint is disabled in Workloadmeta
func TestECSV4Disabled(t *testing.T) {
	testECS(false, t)
}

func testECS(v4 bool, t *testing.T) {
	check, store, sender := prepareTest(t, v4, "ecs")

	// add 2 tasks to fake workloadmetaStore
	task1Id := "123"
	task2Id := "124"
	store.AddECSTasks(task(v4, task1Id))
	store.AddECSTasks(task(v4, task2Id))

	err := check.Run()
	require.NoError(t, err)

	// should receive one message
	messages := sender.Flush()
	require.Len(t, messages, 1)

	groupID := int32(1)
	expectedTasks := expected(v4, groupID, task1Id, task2Id)
	require.Equal(t, expectedTasks, messages[0])
	require.Equal(t, expectedTasks.ClusterId, sender.clusterIDs[0])
	require.Equal(t, orchestrator.ECSTask, sender.nodeTypes[0])

	// add another task with different id to fake workloadmetaStore
	task3Id := "125"
	store.AddECSTasks(task(v4, task3Id))

	err = check.Run()
	require.NoError(t, err)

	messages = sender.Flush()
	require.Len(t, messages, 1)

	groupID++
	require.Equal(t, expected(v4, groupID, task3Id), messages[0])
	require.Equal(t, sender.clusterIDs[0], sender.clusterIDs[1])
	require.Equal(t, sender.nodeTypes[0], sender.nodeTypes[1])

	// 0 message should be received as tasks are skipped by cache
	err = check.Run()
	require.NoError(t, err)
	messages = sender.Flush()
	require.Len(t, messages, 0)
}

// prepareTest returns a check, a fake workloadmeta store and a fake sender
func prepareTest(t *testing.T, v4 bool, env string) (*Check, *fakeWorkloadmetaStore, *fakeSender) {
	t.Helper()

	orchConfig := oconfig.NewDefaultOrchestratorConfig()
	orchConfig.OrchestrationCollectionEnabled = true

	store := &fakeWorkloadmetaStore{
		EnableV4: v4,
	}
	sender := &fakeSender{}

	systemInfo, _ := checks.CollectSystemInfo()

	tagger := nooptagger.NewTaggerClient()

	c := &Check{
		sender:            sender,
		workloadmetaStore: store,
		tagger:            tagger,
		config:            orchConfig,
		groupID:           atomic.NewInt32(0),
		systemInfo:        systemInfo,
	}

	c.isECSCollectionEnabledFunc = func() bool { return false }
	if env == "ecs" {
		c.isECSCollectionEnabledFunc = func() bool { return true }
	}

	return c, store, sender
}

func task(v4 bool, id string) *workloadmeta.ECSTask {
	ecsTask := &workloadmeta.ECSTask{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindECSTask,
			ID:   fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task/%s", id),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: fmt.Sprintf("12345678-1234-1234-1234-123456789%s", id),
		},
		ClusterName: "ecs-cluster",
		LaunchType:  workloadmeta.ECSLaunchTypeEC2,
		Family:      "redis",
		Version:     "1",
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID: "938f6d263c464aa5985dc67ab7f38a7e-1714341083",
			},
			{
				ID: "938f6d263c464aa5985dc67ab7f38a7e-1714341084",
			},
		},
	}

	if v4 {
		ecsTask.AWSAccountID = 123456789012
		ecsTask.Region = "us-east-1"
		ecsTask.DesiredStatus = "RUNNING"
		ecsTask.KnownStatus = "RUNNING"
		ecsTask.VPCID = "vpc-12345678"
		ecsTask.ServiceName = "redis"
		ecsTask.Limits = map[string]float64{"CPU": 1, "Memory": 2048}
		ecsTask.Tags = workloadmeta.MapTags{
			"ecs.cluster": "ecs-cluster",
			"region":      "us-east-1",
		}
		ecsTask.ContainerInstanceTags = workloadmeta.MapTags{
			"instance": "instance-1",
			"region":   "us-east-1",
		}
	}
	return ecsTask
}

func container1(v4 bool) *workloadmeta.Container {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "938f6d263c464aa5985dc67ab7f38a7e-1714341083",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "log_router",
			Labels: map[string]string{
				"com.amazonaws.ecs.cluster":        "ecs-cluster",
				"com.amazonaws.ecs.container-name": "log_router",
			},
		},
		Image: workloadmeta.ContainerImage{
			RawName: "amazon/aws-for-fluent-bit:latest",
			Name:    "amazon/aws-for-fluent-bit",
		},
		Ports: []workloadmeta.ContainerPort{
			{
				Port:     80,
				HostPort: 80,
			},
		},
	}
	if v4 {
		container.ECSContainer = &workloadmeta.ECSContainer{
			DisplayName: "log_router_container",
			Health: &workloadmeta.ContainerHealthStatus{
				Status:   "HEALTHY",
				ExitCode: pointer.Ptr(int64(-2)),
			},
			Type: "NORMAL",
		}
	}
	return container
}

func container2(v4 bool) *workloadmeta.Container {
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   "938f6d263c464aa5985dc67ab7f38a7e-1714341084",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "redis",
		},
	}

	if v4 {
		container.ECSContainer = &workloadmeta.ECSContainer{
			DisplayName: "redis",
			Type:        "NORMAL",
		}
	}
	return container
}

func expected(v4 bool, groupID int32, ids ...string) *process.CollectorECSTask {
	tasks := make([]*process.ECSTask, 0, len(ids))
	for _, id := range ids {
		container1 := &process.ECSContainer{
			DockerID:   "938f6d263c464aa5985dc67ab7f38a7e-1714341083",
			DockerName: "log_router",
			Image:      "amazon/aws-for-fluent-bit:latest",
			Ports: []*process.ECSContainerPort{
				{
					ContainerPort: 80,
					HostPort:      80,
				},
			},
			Labels: []string{
				"com.amazonaws.ecs.cluster:ecs-cluster",
				"com.amazonaws.ecs.container-name:log_router",
			},
		}
		container2 := &process.ECSContainer{
			DockerID:   "938f6d263c464aa5985dc67ab7f38a7e-1714341084",
			DockerName: "redis",
		}

		newTask := &process.ECSTask{
			Arn:        fmt.Sprintf("arn:aws:ecs:us-east-1:123456789012:task/%s", id),
			LaunchType: "ec2",
			Family:     "redis",
			Version:    "1",
			Containers: []*process.ECSContainer{container1, container2},
		}

		if v4 {
			newTask.DesiredStatus = "RUNNING"
			newTask.KnownStatus = "RUNNING"
			newTask.VpcId = "vpc-12345678"
			newTask.ServiceName = "redis"
			newTask.Limits = map[string]float64{"CPU": 1, "Memory": 2048}
			newTask.EcsTags = []string{
				"ecs.cluster:ecs-cluster",
				"region:us-east-1",
			}
			newTask.ContainerInstanceTags = []string{
				"instance:instance-1",
				"region:us-east-1",
			}

			container1.Name = "log_router_container"
			container1.Type = "NORMAL"
			container1.Health = &process.ECSContainerHealth{
				Status: "HEALTHY",
				ExitCode: &process.ECSContainerExitCode{
					ExitCode: -2,
				},
			}

			container2.Name = "redis"
			container2.Type = "NORMAL"
		}

		newTask.ResourceVersion = ecs.BuildTaskResourceVersion(newTask)
		tasks = append(tasks, newTask)
	}

	systemInfo, _ := checks.CollectSystemInfo()

	return &process.CollectorECSTask{
		AwsAccountID: 123456789012,
		ClusterName:  "ecs-cluster",
		ClusterId:    "63306530-3932-3664-3664-376566306132",
		Region:       "us-east-1",
		GroupId:      groupID,
		GroupSize:    1,
		Tasks:        tasks,
		Info:         systemInfo,
	}
}
