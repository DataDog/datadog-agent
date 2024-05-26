// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package vm

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var activeContainerWithoutProperties = gardenfakes.FakeContainer{
	HandleStub: func() string {
		return "container-handle-0"
	},
	InfoStub: func() (garden.ContainerInfo, error) {
		return garden.ContainerInfo{
			State:       "active",
			Events:      nil,
			HostIP:      "container-host-ip-0",
			ContainerIP: "container-ip-0",
			ExternalIP:  "container-external-ip-0",
			Properties:  garden.Properties{},
		}, nil
	},
}

var activeContainerWithProperties = gardenfakes.FakeContainer{
	HandleStub: func() string {
		return "container-handle-1"
	},
	InfoStub: func() (garden.ContainerInfo, error) {
		return garden.ContainerInfo{
			State:       "active",
			Events:      nil,
			HostIP:      "container-host-ip-1",
			ContainerIP: "container-ip-1",
			ExternalIP:  "container-external-ip-1",
			Properties: garden.Properties{
				"log_config": "{\"guid\":\"app-guid-1\",\"index\":0,\"source_name\":\"CELL\",\"tags\":{\"app_id\":\"app-id-1\",\"app_name\":\"app-name-1\"}}",
			},
		}, nil
	},
}

var stoppedContainer = gardenfakes.FakeContainer{
	HandleStub: func() string {
		return "container-handle-2"
	},
	InfoStub: func() (garden.ContainerInfo, error) {
		return garden.ContainerInfo{
			State:       "stopped",
			Events:      nil,
			HostIP:      "container-host-ip-2",
			ContainerIP: "container-ip-2",
			ExternalIP:  "container-external-ip-2",
			Properties:  garden.Properties{},
		}, nil
	},
}

type FakeGardenUtil struct {
	containers []garden.Container
}

func (f *FakeGardenUtil) ListContainers() ([]garden.Container, error) {
	return f.containers, nil
}
func (f *FakeGardenUtil) GetContainersInfo(handles []string) (map[string]garden.ContainerInfoEntry, error) {
	containersInfo := make(map[string]garden.ContainerInfoEntry)
	for _, container := range f.containers {
		handle := container.Handle()
		if !slices.Contains(handles, handle) {
			continue
		}
		info, err := container.Info()
		entry := garden.ContainerInfoEntry{
			Info: info,
		}
		if err != nil {
			entry.Err = &garden.Error{Err: err}
		}
		containersInfo[handle] = entry
	}
	return containersInfo, nil
}

func (f *FakeGardenUtil) GetContainersMetrics(_ []string) (map[string]garden.ContainerMetricsEntry, error) {
	return map[string]garden.ContainerMetricsEntry{}, nil

}
func (f *FakeGardenUtil) GetContainer(handle string) (garden.Container, error) {
	for _, container := range f.containers {
		if container.Handle() == handle {
			return container, nil
		}
	}
	return nil, fmt.Errorf("container '%s' not found", handle)
}

type FakeDCAClient struct {
	LocalVersion                 version.Version
	LocalClusterAgentAPIEndpoint string

	VersionErr error

	NodeLabels    map[string]string
	NodeLabelsErr error

	NodeAnnotations    map[string]string
	NodeAnnotationsErr error

	NamespaceLabels    map[string]string
	NamespaceLabelsErr error

	PodMetadataForNode    apiv1.NamespacesPodsStringsSet
	PodMetadataForNodeErr error

	KubernetesMetadataNames    []string
	KubernetesMetadataNamesErr error

	ClusterCheckStatus    types.StatusResponse
	ClusterCheckStatusErr error

	ClusterCheckConfigs    types.ConfigResponse
	ClusterCheckConfigsErr error

	EndpointsCheckConfigs    types.ConfigResponse
	EndpointsCheckConfigsErr error

	ClusterID    string
	ClusterIDErr error
}

func (f *FakeDCAClient) Version() version.Version {
	panic("implement me")
}

func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
	panic("implement me")
}

func (f *FakeDCAClient) GetVersion() (version.Version, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNodeLabels(_ string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNodeAnnotations(_ string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNamespaceLabels(_ string) (map[string]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetNamespaceMetadata(_ string) (*clusteragent.Metadata, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetPodsMetadataForNode(_ string) (apiv1.NamespacesPodsStringsSet, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetKubernetesMetadataNames(_, _, _ string) ([]string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) PostClusterCheckStatus(_ context.Context, _ string, _ types.NodeStatus) (types.StatusResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetClusterCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetEndpointsCheckConfigs(_ context.Context, _ string) (types.ConfigResponse, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetKubernetesClusterID() (string, error) {
	panic("implement me")
}

func (f *FakeDCAClient) GetCFAppsMetadataForNode(_ string) (map[string][]string, error) {
	return map[string][]string{
		activeContainerWithoutProperties.Handle(): {"container_name:active-container-app"},
		stoppedContainer.Handle():                 {"container_name:stopped-container-app"},
	}, nil
}

func (f *FakeDCAClient) PostLanguageMetadata(_ context.Context, _ *pbgo.ParentLanguageAnnotationRequest) error {
	panic("implement me")
}

func (f *FakeDCAClient) SupportsNamespaceMetadataCollection() bool {
	panic("implement me")
}

func TestStartError(t *testing.T) {
	fakeGardenUtil := FakeGardenUtil{}

	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
	}

	err := c.Start(context.TODO(), workloadmetaStore)
	assert.Error(t, err)
}

func TestPullNoContainers(t *testing.T) {
	fakeGardenUtil := FakeGardenUtil{containers: nil}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.Empty(t, evs)
}

func TestPullActiveContainer(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]

	assert.Equal(t, event0.Type, workloadmeta.EventTypeSet)
	assert.Equal(t, event0.Source, workloadmeta.SourceClusterOrchestrator)

	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)

	assert.Equal(t, containerEntity.Kind, workloadmeta.KindContainer)
	assert.Equal(t, containerEntity.Runtime, workloadmeta.ContainerRuntimeGarden)
	assert.Equal(t, containerEntity.ID, activeContainerWithoutProperties.Handle())
	assert.Equal(t, containerEntity.State.Status, workloadmeta.ContainerStatusRunning)
	assert.True(t, containerEntity.State.Running)
	assert.NotEmpty(t, containerEntity.CollectorTags)
}

func TestPullStoppedContainer(t *testing.T) {
	containers := []garden.Container{
		&stoppedContainer,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]

	assert.Equal(t, event0.Type, workloadmeta.EventTypeSet)
	assert.Equal(t, event0.Source, workloadmeta.SourceClusterOrchestrator)

	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)

	assert.Equal(t, containerEntity.Kind, workloadmeta.KindContainer)
	assert.Equal(t, containerEntity.Runtime, workloadmeta.ContainerRuntimeGarden)
	assert.Equal(t, containerEntity.ID, stoppedContainer.Handle())
	assert.Equal(t, containerEntity.State.Status, workloadmeta.ContainerStatusStopped)
	assert.False(t, containerEntity.State.Running)
	assert.NotEmpty(t, containerEntity.CollectorTags)
}

func TestPullDetectsDeletedContainers(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	// expect a set event of the active container
	event0 := evs[0]

	assert.Equal(t, event0.Type, workloadmeta.EventTypeSet)
	assert.Equal(t, event0.Source, workloadmeta.SourceClusterOrchestrator)

	// remove containers
	fakeGardenUtil.containers = nil

	err = c.Pull(context.TODO())
	assert.NoError(t, err)

	evs = workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	// expect an unset event of the previous active container
	event1 := evs[1]
	containerEntity, ok := event1.Entity.(*workloadmeta.Container)

	assert.True(t, ok)

	assert.Equal(t, event1.Type, workloadmeta.EventTypeUnset)
	assert.Equal(t, event1.Source, workloadmeta.SourceClusterOrchestrator)
	assert.Equal(t, containerEntity.Kind, workloadmeta.KindContainer)
	assert.Equal(t, containerEntity.ID, activeContainerWithoutProperties.Handle())
}

func TestPullAppNameWithDCA(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]
	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)
	assert.Contains(t, containerEntity.CollectorTags, "container_name:active-container-app")
}

func TestPullNoAppNameWithoutDCA(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}

	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: false, // disabled DCA
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]
	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)
	assert.Contains(t, containerEntity.CollectorTags, fmt.Sprintf("container_name:%s", activeContainerWithoutProperties.Handle()))
}

func TestPullAppNameWithGardenPropertiesWithoutDCA(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}

	// We do not inject any collectors here; we instantiate
	// and initialize it out-of-band below. That's OK.
	workloadmetaStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModule(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: false, // disabled DCA
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	evs := workloadmetaStore.GetNotifiedEvents()
	assert.NotEmpty(t, evs)

	event0 := evs[0]
	containerEntity, ok := event0.Entity.(*workloadmeta.Container)
	assert.True(t, ok)
	assert.Contains(t, containerEntity.CollectorTags, fmt.Sprintf("container_name:%s", "app-name-1"))
}
