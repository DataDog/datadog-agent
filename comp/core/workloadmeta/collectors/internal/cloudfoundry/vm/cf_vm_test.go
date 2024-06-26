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
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var waitFor = 10 * time.Second
var tick = 50 * time.Millisecond

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

func (f *FakeDCAClient) Version(_ bool) version.Version {
	panic("implement me")
}

func (f *FakeDCAClient) ClusterAgentAPIEndpoint() string {
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

	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
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
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	// If pull sets a container in the store, the one here will be added after
	// it because workloadmeta processes the events in order. Therefore, if we
	// only see this container and no others when listing, it means Pull()
	// didn't add anything.
	workloadmetaStore.Set(
		&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "test-container",
			},
		},
	)

	assert.Eventually(t, func() bool {
		containers := workloadmetaStore.ListContainers()
		return len(containers) == 1 && containers[0].ID == "test-container"
	}, waitFor, tick)
}

func TestPullActiveContainer(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
		return err == nil
	}, waitFor, tick)

	container, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
	require.NoError(t, err)

	assert.Equal(t, container.Kind, workloadmeta.KindContainer)
	assert.Equal(t, container.Runtime, workloadmeta.ContainerRuntimeGarden)
	assert.Equal(t, container.ID, activeContainerWithoutProperties.Handle())
	assert.Equal(t, container.State.Status, workloadmeta.ContainerStatusRunning)
	assert.True(t, container.State.Running)
	assert.NotEmpty(t, container.CollectorTags)
}

func TestPullStoppedContainer(t *testing.T) {
	containers := []garden.Container{
		&stoppedContainer,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(stoppedContainer.Handle())
		return err == nil
	}, waitFor, tick)

	container, err := workloadmetaStore.GetContainer(stoppedContainer.Handle())
	require.NoError(t, err)

	assert.Equal(t, container.Kind, workloadmeta.KindContainer)
	assert.Equal(t, container.Runtime, workloadmeta.ContainerRuntimeGarden)
	assert.Equal(t, container.ID, stoppedContainer.Handle())
	assert.Equal(t, container.State.Status, workloadmeta.ContainerStatusStopped)
	assert.False(t, container.State.Running)
	assert.NotEmpty(t, container.CollectorTags)
}

func TestPullDetectsDeletedContainers(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
		return err == nil
	}, waitFor, tick)

	// remove containers
	fakeGardenUtil.containers = nil

	err = c.Pull(context.TODO())
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
		return err != nil
	}, waitFor, tick)
}

func TestPullAppNameWithDCA(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: true,
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
		return err == nil
	}, waitFor, tick)

	container, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
	require.NoError(t, err)

	assert.Contains(t, container.CollectorTags, "container_name:active-container-app")
}

func TestPullNoAppNameWithoutDCA(t *testing.T) {
	containers := []garden.Container{
		&activeContainerWithoutProperties,
	}
	fakeGardenUtil := FakeGardenUtil{
		containers: containers,
	}
	fakeDCAClient := FakeDCAClient{}

	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: false, // disabled DCA
	}

	err := c.Pull(context.TODO())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
		return err == nil
	}, waitFor, tick)

	container, err := workloadmetaStore.GetContainer(activeContainerWithoutProperties.Handle())
	require.NoError(t, err)

	assert.Contains(t, container.CollectorTags, fmt.Sprintf("container_name:%s", activeContainerWithoutProperties.Handle()))
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
	workloadmetaStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmetafxmock.MockModuleV2(),
	))

	c := collector{
		gardenUtil: &fakeGardenUtil,
		store:      workloadmetaStore,
		dcaClient:  &fakeDCAClient,
		dcaEnabled: false, // disabled DCA
	}

	err := c.Pull(context.TODO())
	assert.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := workloadmetaStore.GetContainer(activeContainerWithProperties.Handle())
		return err == nil
	}, waitFor, tick)

	container, err := workloadmetaStore.GetContainer(activeContainerWithProperties.Handle())
	require.NoError(t, err)

	assert.Contains(t, container.CollectorTags, fmt.Sprintf("container_name:%s", "app-name-1"))
}
