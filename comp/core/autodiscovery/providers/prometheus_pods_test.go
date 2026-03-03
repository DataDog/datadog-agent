// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStream(t *testing.T) {
	testPodID := "pod-id"
	testPodName := "pod-name"
	testContainerID := "container-id"
	testContainerName := "container-name"

	wmeta := newMockWorkloadMeta(t)
	providerInterface, err := NewPrometheusPodsConfigProvider(nil, wmeta, nil, nil, nil)
	require.NoError(t, err)

	provider, ok := providerInterface.(*PrometheusPodsConfigProvider)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	configCh := provider.Stream(ctx)

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   testPodID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testPodName,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   testContainerID,
				Name: testContainerName,
			},
		},
	}

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   testContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testContainerName,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}

	wmeta.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: container,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: pod,
		},
	})

	// We should get a first empty changes corresponding to the first event that
	// workloadmeta sends to subscribers when they subscribe.
	changes := <-configCh
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 0)

	// There should be a new config associated with the pod and container
	// created in workloadmeta
	changes = <-configCh
	require.Len(t, changes.Schedule, 1)
	require.Len(t, changes.Unschedule, 0)

	// Verify the scheduled config
	configToSchedule := changes.Schedule[0]
	assert.Equal(t, "openmetrics", configToSchedule.Name)
	assert.Equal(t, names.PrometheusPods, configToSchedule.Provider)
	assert.Equal(t, "prometheus_pods:containerd://"+testContainerID, configToSchedule.Source)
	assert.Equal(t, []string{"containerd://" + testContainerID}, configToSchedule.ADIdentifiers)

	// Remove the pod
	wmeta.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeUnset,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   testPodID,
				},
			},
		},
	})

	// There should be a config to unschedule after the pod is removed
	changes = <-configCh
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 1)

	configToUnschedule := changes.Unschedule[0]
	assert.Equal(t, "openmetrics", configToUnschedule.Name)
	assert.Equal(t, names.PrometheusPods, configToUnschedule.Provider)
	assert.Equal(t, "prometheus_pods:containerd://"+testContainerID, configToUnschedule.Source)
	assert.Equal(t, []string{"containerd://" + testContainerID}, configToUnschedule.ADIdentifiers)
}

func TestStream_NoAnnotations(t *testing.T) {
	testPodID := "pod-id"
	testPodName := "pod-name"
	testContainerID := "container-id"
	testContainerName := "container-name"

	wmeta := newMockWorkloadMeta(t)
	providerInterface, err := NewPrometheusPodsConfigProvider(nil, wmeta, nil, nil, nil)
	require.NoError(t, err)

	provider, ok := providerInterface.(*PrometheusPodsConfigProvider)
	require.True(t, ok)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	configCh := provider.Stream(ctx)

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   testPodID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        testPodName,
			Annotations: map[string]string{}, // No prometheus annotations
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{
				ID:   testContainerID,
				Name: testContainerName,
			},
		},
	}

	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   testContainerID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testContainerName,
		},
		Runtime: workloadmeta.ContainerRuntimeContainerd,
	}

	wmeta.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: container,
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: pod,
		},
	})

	// We should get a first empty changes corresponding to the first event that
	// workloadmeta sends to subscribers when they subscribe.
	changes := <-configCh
	require.Len(t, changes.Schedule, 0)
	require.Len(t, changes.Unschedule, 0)

	// Get config changes - should be empty since no prometheus annotations
	changes = <-configCh
	assert.Len(t, changes.Schedule, 0)
	assert.Len(t, changes.Unschedule, 0)
}

func TestGetConfigErrors(t *testing.T) {
	tests := []struct {
		name          string
		events        []workloadmeta.Event
		pods          []*workloadmeta.KubernetesPod
		initialErrors map[string]providerTypes.ErrorMsgSet
		wantErrs      bool
	}{
		{
			name: "pod with invalid port annotation",
			events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "pod-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "pod-name",
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "invalid",
							},
						},
						Containers: []workloadmeta.OrchestratorContainer{
							{
								ID:   "container-id",
								Name: "container-name",
							},
						},
					},
				},
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   "container-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "container-name",
						},
						Runtime: workloadmeta.ContainerRuntimeContainerd,
						Ports: []workloadmeta.ContainerPort{
							{
								Port: 8080,
							},
						},
					},
				},
			},
			wantErrs: true,
		},
		{
			name: "pod with port annotation but no matching container should not generate errors",
			events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "pod-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "pod-name",
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
								"prometheus.io/port":   "9999",
							},
						},
						Containers: []workloadmeta.OrchestratorContainer{
							{
								ID:   "container-id",
								Name: "container-name",
							},
						},
					},
				},
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   "container-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "container-name",
						},
						Runtime: workloadmeta.ContainerRuntimeContainerd,
						Ports: []workloadmeta.ContainerPort{
							{
								Port: 8080, // Doesn't match port in annotation
							},
						},
					},
				},
			},
			wantErrs: false,
		},
		{
			name: "valid pod should not generate errors",
			events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "pod-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "pod-name",
							Annotations: map[string]string{
								"prometheus.io/scrape": "true",
							},
						},
						Containers: []workloadmeta.OrchestratorContainer{
							{
								ID:   "container-id",
								Name: "container-name",
							},
						},
					},
				},
				{
					Type: workloadmeta.EventTypeSet,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   "container-id",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "container-name",
						},
						Runtime: workloadmeta.ContainerRuntimeContainerd,
						Ports: []workloadmeta.ContainerPort{
							{
								Port: 8080,
							},
						},
					},
				},
			},
			wantErrs: false,
		},
		{
			name: "pod unset event",
			events: []workloadmeta.Event{
				{
					Type: workloadmeta.EventTypeUnset,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "pod-id",
						},
					},
				},
				{
					Type: workloadmeta.EventTypeUnset,
					Entity: &workloadmeta.Container{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindContainer,
							ID:   "container-id",
						},
					},
				},
			},
			initialErrors: map[string]providerTypes.ErrorMsgSet{
				"pod-id": {
					"some-error": struct{}{},
				},
			},
			wantErrs: false, // The initial errors should be deleted
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wmeta := newMockWorkloadMeta(t)
			providerInterface, err := NewPrometheusPodsConfigProvider(nil, wmeta, nil, nil, nil)
			require.NoError(t, err)

			provider, ok := providerInterface.(*PrometheusPodsConfigProvider)
			require.True(t, ok)

			eventBundle := workloadmeta.EventBundle{
				Events: test.events,
				Ch:     make(chan struct{}),
			}

			_ = provider.processEvents(eventBundle)

			errors := provider.GetConfigErrors()

			if test.wantErrs {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

func newMockWorkloadMeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](
		t,
		fx.Options(
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return config.NewMock(t) }),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		),
	)
}
