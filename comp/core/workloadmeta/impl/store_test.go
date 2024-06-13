// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package workloadmeta

import (
	"reflect"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"gotest.tools/assert" //nolint:depguard

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	dummySubscriber = "subscriber"
	fooSource       = "foo"
	barSource       = "bar"
)

func newWorkloadmetaObject(deps dependencies) *workloadmeta {
	return NewWorkloadMeta(deps).Comp.(*workloadmeta)
}

func TestHandleEvents(t *testing.T) {

	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "deadbeef",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})

	gotContainer, err := s.GetContainer(container.ID)
	if err != nil {
		t.Errorf("expected to find container %q, not found", container.ID)
	}

	if !reflect.DeepEqual(container, gotContainer) {
		t.Errorf("expected container %q to match the one in the store", container.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: container,
		},
	})

	_, err = s.GetContainer(container.ID)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected container %q to be absent. found or had errors. err: %q", container.ID, err)
	}
}

func TestSubscribe(t *testing.T) {
	fooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo-name-might-be-overridden",
		},
		Hostname: "foo",
	}

	fooContainerToMerge := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo-name-override",
		},
		PID: 1001001,
	}

	fooContainerMerged := &wmdef.Container{
		EntityID: fooContainer.EntityID,
		EntityMeta: wmdef.EntityMeta{
			Name: fooContainerToMerge.Name,
		},
		Hostname: fooContainer.Hostname,
		PID:      fooContainerToMerge.PID,
	}

	barContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "bar",
		},
	}

	bazContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "baz",
		},
	}

	tests := []struct {
		name       string
		preEvents  []wmdef.CollectorEvent
		postEvents [][]wmdef.CollectorEvent
		filter     *wmdef.Filter
		expected   []wmdef.EventBundle
	}{
		{
			// will receive events for entities that are currently
			// in the store. entities that were deleted before the
			// subscription should not generate events.
			name: "receive events for entities in the store pre-subscription",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   wmdef.EventTypeUnset,
					Source: fooSource,
					Entity: barContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// if the filter has type "wmdef.EventTypeUnset", it does not receive
			// events for entities that are currently in the store.
			name:   "do not receive events for entities in the store pre-subscription if filter type is EventTypeUnset",
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).SetEventType(wmdef.EventTypeUnset).Build(),
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{},
			},
		},
		{
			// will receive events for entities that are currently
			// in the store, and match a filter by source. entities
			// that don't match the filter at all should not
			// generate an event.
			name:   "receive events for entities in the store pre-subscription with filter",
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).Build(),
			preEvents: []wmdef.CollectorEvent{
				// set container with two sources, delete one source
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeUnset,
					Source: barSource,
					Entity: fooContainer,
				},

				// set container with two sources, keep them
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: barContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: barContainer,
				},

				// set a container for source that should be
				// filtered out
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: bazContainer,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: barContainer,
						},
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// same as previous test, but now after the subscription started
			name: "merges entities from different sources post-subscription",
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainerMerged,
						},
					},
				},
			},
		},
		{
			// an event about an entity generated from two
			// different sources gets merged into a single entity
			// containing data from both events
			name: "merges entities from different sources pre-subscription",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: fooContainer,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: barSource,
					Entity: fooContainerToMerge,
				},
			},
			expected: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainerMerged,
						},
					},
				},
			},
		},
		{
			name: "sets and unsets an entity",
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// setting an entity from two different sources, but
			// unsetting from only one (that matches the filter)
			// correctly generates an unset event
			name:   "sets and unsets an entity with source filters",
			filter: wmdef.NewFilterBuilder().SetSource(fooSource).Build(),
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// setting an entity from two different sources, and
			// unsetting one of them, correctly generates a three
			// sets and no unsets
			name:   "sets and unsets an entity from different sources",
			filter: nil,
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: barSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			// unsetting an unknown entity should generate no events
			name:   "unsets unknown entity",
			filter: nil,
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
			},
		},
		{
			// unsetting an entity with a non-empty state (as in,
			// emitting data in other fields instead of just a
			// wrapped EntityID) merges that with the last known
			// state of the entity before deletion.
			name:   "unsetting entity merges last known state",
			filter: nil,
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainerToMerge,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
						{
							Type: wmdef.EventTypeUnset,
							Entity: &wmdef.Container{
								EntityID: fooContainer.EntityID,
								EntityMeta: wmdef.EntityMeta{
									Name: fooContainer.Name,
								},
								Hostname: fooContainer.Hostname,
								PID:      fooContainerToMerge.PID,
							},
						},
					},
				},
			},
		},
		{
			name:   "filters by event type",
			filter: wmdef.NewFilterBuilder().SetEventType(wmdef.EventTypeUnset).Build(),
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
				{
					{
						Type:   wmdef.EventTypeUnset,
						Source: fooSource,
						Entity: fooContainer,
					},
				},
			},
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			name:      "sets unchanged entity twice",
			preEvents: []wmdef.CollectorEvent{},
			postEvents: [][]wmdef.CollectorEvent{
				{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: fooContainer,
					},
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						// DeepCopy to ensure we're not
						// just comparing pointers, as
						// collectors return a freshly
						// built object every time
						Entity: fooContainer.DeepCopy(),
					},
				},
			},
			filter: nil,
			expected: []wmdef.EventBundle{
				{},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(tt.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, tt.filter)
			doneCh := make(chan struct{})

			actual := []wmdef.EventBundle{}
			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

					// nil the bundle's Ch so we can
					// deep-equal just the events later
					bundle.Ch = nil

					actual = append(actual, bundle)
				}

				close(doneCh)
			}()

			for _, events := range tt.postEvents {
				s.handleEvents(events)
			}

			s.Unsubscribe(ch)

			<-doneCh
			tassert.Equal(t, tt.expected, actual)
			tassert.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetKubernetesDeployment(t *testing.T) {
	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	deployment := &wmdef.KubernetesDeployment{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesDeployment,
			ID:   "datadog-cluster-agent",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: deployment,
		},
	})

	retrievedDeployment, err := s.GetKubernetesDeployment("datadog-cluster-agent")
	tassert.NoError(t, err)

	if !reflect.DeepEqual(deployment, retrievedDeployment) {
		t.Errorf("expected deployment %q to match the one in the store", retrievedDeployment.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: deployment,
		},
	})

	_, err = s.GetKubernetesDeployment("datadog-cluster-agent")
	tassert.True(t, errors.IsNotFound(err))
}

func TestGetProcess(t *testing.T) {
	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	process := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: process,
		},
	})

	gotProcess, err := s.GetProcess(123)
	if err != nil {
		t.Errorf("expected to find process %q, not found", process.ID)
	}

	if !reflect.DeepEqual(process, gotProcess) {
		t.Errorf("expected process %q to match the one in the store", process.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: process,
		},
	})

	_, err = s.GetProcess(123)
	if err == nil || !errors.IsNotFound(err) {
		t.Errorf("expected process %q to be absent. found or had errors. err: %q", process.ID, err)
	}
}

func TestListContainers(t *testing.T) {
	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "abc",
		},
	}

	tests := []struct {
		name               string
		preEvents          []wmdef.CollectorEvent
		expectedContainers []*wmdef.Container
	}{
		{
			name: "some containers stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: container,
				},
			},
			expectedContainers: []*wmdef.Container{container},
		},
		{
			name:               "no containers stored",
			preEvents:          nil,
			expectedContainers: []*wmdef.Container{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			containers := s.ListContainers()

			tassert.Equal(t, test.expectedContainers, containers)
		})
	}
}

func TestListContainersWithFilter(t *testing.T) {
	runningContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "1",
		},
		State: wmdef.ContainerState{
			Running: true,
		},
	}

	nonRunningContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "2",
		},
		State: wmdef.ContainerState{
			Running: false,
		},
	}

	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: runningContainer,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: nonRunningContainer,
		},
	})

	runningContainers := s.ListContainersWithFilter(wmdef.GetRunningContainers)

	tassert.Equal(t, []*wmdef.Container{runningContainer}, runningContainers)
}

func TestListProcesses(t *testing.T) {
	process := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
	}

	tests := []struct {
		name              string
		preEvents         []wmdef.CollectorEvent
		expectedProcesses []*wmdef.Process
	}{
		{
			name: "some processes stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: process,
				},
			},
			expectedProcesses: []*wmdef.Process{process},
		},
		{
			name:              "no processes stored",
			preEvents:         nil,
			expectedProcesses: []*wmdef.Process{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			processes := s.ListProcesses()

			tassert.Equal(t, test.expectedProcesses, processes)
		})
	}
}

func TestListProcessesWithFilter(t *testing.T) {
	javaProcess := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "123",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Java,
		},
	}

	nodeProcess := &wmdef.Process{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindProcess,
			ID:   "2",
		},
		Language: &languagemodels.Language{
			Name: languagemodels.Node,
		},
	}

	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: javaProcess,
		},
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: nodeProcess,
		},
	})

	retrievedProcesses := s.ListProcessesWithFilter(func(p *wmdef.Process) bool {
		return p.Language.Name == languagemodels.Java
	})

	tassert.Equal(t, []*wmdef.Process{javaProcess}, retrievedProcesses)
}

func TestGetKubernetesPodByName(t *testing.T) {
	pod1 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "123",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}
	pod2 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "234",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod-other",
			Namespace: "test-namespace",
		},
	}
	pod3 := &wmdef.KubernetesPod{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesPod,
			ID:   "345",
		},
		EntityMeta: wmdef.EntityMeta{
			Name:      "test-pod",
			Namespace: "test-namespace-other",
		},
	}

	type want struct {
		pod *wmdef.KubernetesPod
		err error
	}
	type args struct {
		podName      string
		podNamespace string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "test-pod/test-namespace returns correct pod",
			args: args{
				podName:      "test-pod",
				podNamespace: "test-namespace",
			},
			want: want{
				pod: pod1,
			},
		},
		{
			name: "test-pod-other/test-namespace returns correct pod",
			args: args{
				podName:      "test-pod-other",
				podNamespace: "test-namespace",
			},
			want: want{
				pod: pod2,
			},
		},
		{
			name: "test-pod/test-namespace-other returns correct pod",
			args: args{
				podName:      "test-pod",
				podNamespace: "test-namespace-other",
			},
			want: want{
				pod: pod3,
			},
		},
		{
			name: "missing pod returns error",
			args: args{
				podName:      "test-pod-other",
				podNamespace: "test-namespace-other",
			},
			want: want{
				err: errors.NewNotFound("test-pod-other"),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			for _, pod := range []*wmdef.KubernetesPod{pod1, pod2, pod3} {
				s.handleEvents([]wmdef.CollectorEvent{
					{
						Type:   wmdef.EventTypeSet,
						Source: fooSource,
						Entity: pod,
					},
				})
			}

			pod, err := s.GetKubernetesPodByName(test.args.podName, test.args.podNamespace)

			tassert.Equal(t, test.want.pod, pod)
			if test.want.err != nil {
				assert.Error(t, err, test.want.err.Error())
			}
		})
	}
}

func TestListKubernetesNodes(t *testing.T) {
	node := &wmdef.KubernetesNode{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesNode,
			ID:   "some-node",
		},
	}

	tests := []struct {
		name          string
		preEvents     []wmdef.CollectorEvent
		expectedNodes []*wmdef.KubernetesNode
	}{
		{
			name: "some nodes stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: node,
				},
			},
			expectedNodes: []*wmdef.KubernetesNode{node},
		},
		{
			name:          "no nodes stored",
			preEvents:     nil,
			expectedNodes: []*wmdef.KubernetesNode{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))
			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			tassert.Equal(t, test.expectedNodes, s.ListKubernetesNodes())
		})
	}
}

func TestListImages(t *testing.T) {
	image := &wmdef.ContainerImageMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name           string
		preEvents      []wmdef.CollectorEvent
		expectedImages []*wmdef.ContainerImageMetadata
	}{
		{
			name: "some images stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: image,
				},
			},
			expectedImages: []*wmdef.ContainerImageMetadata{image},
		},
		{
			name:           "no containers stored",
			preEvents:      nil,
			expectedImages: []*wmdef.ContainerImageMetadata{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			assert.DeepEqual(t, test.expectedImages, s.ListImages())
		})
	}
}

func TestGetImage(t *testing.T) {
	image := &wmdef.ContainerImageMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainerImageMetadata,
			ID:   "abc",
		},
	}

	tests := []struct {
		name          string
		imageID       string
		preEvents     []wmdef.CollectorEvent
		expectedImage *wmdef.ContainerImageMetadata
		expectsError  bool
	}{
		{
			name:    "image exists",
			imageID: image.ID,
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: image,
				},
			},
			expectedImage: image,
		},
		{
			name:         "image does not exist",
			imageID:      "non_existing_ID",
			preEvents:    nil,
			expectsError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)
			s.handleEvents(test.preEvents)

			actualImage, err := s.GetImage(test.imageID)

			if test.expectsError {
				tassert.Error(t, err, errors.NewNotFound(string(wmdef.KindContainerImageMetadata)).Error())
			} else {
				tassert.NoError(t, err)
				tassert.Equal(t, test.expectedImage, actualImage)
			}
		})
	}
}

func TestListECSTasks(t *testing.T) {
	task1 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-1",
		},
		VPCID: "123",
	}
	task2 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-1",
		},
	}
	task3 := &wmdef.ECSTask{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindECSTask,
			ID:   "task-id-2",
		},
	}

	tests := []struct {
		name          string
		preEvents     []wmdef.CollectorEvent
		expectedTasks []*wmdef.ECSTask
	}{
		{
			name: "some tasks stored",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task1,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task2,
				},
				{
					Type:   wmdef.EventTypeSet,
					Source: fooSource,
					Entity: task3,
				},
			},
			// task2 replaces task1
			expectedTasks: []*wmdef.ECSTask{task2, task3},
		},
		{
			name:          "no task stored",
			preEvents:     nil,
			expectedTasks: []*wmdef.ECSTask{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			tasks := s.ListECSTasks()

			tassert.ElementsMatch(t, test.expectedTasks, tasks)
		})
	}
}

func TestResetProcesses(t *testing.T) {
	tests := []struct {
		name         string
		preEvents    []wmdef.CollectorEvent
		newProcesses []*wmdef.Process
	}{
		{
			name:      "initially empty",
			preEvents: []wmdef.CollectorEvent{},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
				},
			},
		},
		{
			name: "old process to be removed",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: wmdef.SourceRemoteProcessCollector,
					Entity: &wmdef.Process{
						EntityID: wmdef.EntityID{
							Kind: wmdef.KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "345",
					},
				},
			},
		},
		{
			name: "old process to be updated",
			preEvents: []wmdef.CollectorEvent{
				{
					Type:   wmdef.EventTypeSet,
					Source: wmdef.SourceRemoteProcessCollector,
					Entity: &wmdef.Process{
						EntityID: wmdef.EntityID{
							Kind: wmdef.KindProcess,
							ID:   "123",
						},
					},
				},
			},
			newProcesses: []*wmdef.Process{
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "123",
					},
					NsPid: 345,
				},
				{
					EntityID: wmdef.EntityID{
						Kind: wmdef.KindProcess,
						ID:   "12",
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)
			s.handleEvents(test.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
			doneCh := make(chan struct{})

			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

					// nil the bundle's Ch so we can deep-equal just the events
					// later
					bundle.Ch = nil
				}

				close(doneCh)
			}()

			entities := make([]wmdef.Entity, len(test.newProcesses))
			for i := range test.newProcesses {
				entities[i] = test.newProcesses[i]
			}
			s.ResetProcesses(entities, wmdef.SourceRemoteProcessCollector)
			// Force handling of events generated by the reset
			if len(s.eventCh) > 0 {
				s.handleEvents(<-s.eventCh)
			}

			s.Unsubscribe(ch)

			<-doneCh

			processes := s.ListProcesses()
			tassert.ElementsMatch(t, processes, test.newProcesses)
		})
	}

}

func TestGetKubernetesMetadata(t *testing.T) {
	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	kubemetadata := &wmdef.KubernetesMetadata{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindKubernetesMetadata,
			ID:   "deployments/default/app",
		},
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: kubemetadata,
		},
	})

	retrievedMetadata, err := s.GetKubernetesMetadata("deployments/default/app")
	tassert.NoError(t, err)

	if !reflect.DeepEqual(kubemetadata, retrievedMetadata) {
		t.Errorf("expected metadata %q to match the one in the store", retrievedMetadata.ID)
	}

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeUnset,
			Source: fooSource,
			Entity: kubemetadata,
		},
	})

	_, err = s.GetKubernetesMetadata("deployments/default/app")
	tassert.True(t, errors.IsNotFound(err))
}

func TestReset(t *testing.T) {
	fooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo",
		},
		Hostname: "foo",
	}

	fooSetEvent := wmdef.CollectorEvent{
		Type:   wmdef.EventTypeSet,
		Source: fooSource,
		Entity: fooContainer,
	}

	// Same ID as fooContainer but with different values
	updatedFooContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "foo",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "foo",
			Labels: map[string]string{ // Added
				"test-label": "1",
			},
		},
		Hostname: "foo",
	}

	barContainer := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "bar",
		},
		EntityMeta: wmdef.EntityMeta{
			Name: "bar",
		},
		Hostname: "bar",
	}

	tests := []struct {
		name                   string
		preEvents              []wmdef.CollectorEvent
		newEntities            []wmdef.Entity
		expectedEventsReceived []wmdef.EventBundle
	}{
		{
			name: "new entity already exists without changes",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				fooContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
			},
		},
		{
			name: "new entity exists but it has been updated",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				updatedFooContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: updatedFooContainer,
						},
					},
				},
			},
		},
		{
			name: "new event does not exist",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{
				fooContainer,
				barContainer,
			},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: barContainer,
						},
					},
				},
			},
		},
		{
			name: "an event that exists is not included in the list of new ones",
			preEvents: []wmdef.CollectorEvent{
				fooSetEvent,
			},
			newEntities: []wmdef.Entity{},
			expectedEventsReceived: []wmdef.EventBundle{
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeSet,
							Entity: fooContainer,
						},
					},
				},
				{
					Events: []wmdef.Event{
						{
							Type:   wmdef.EventTypeUnset,
							Entity: fooContainer,
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				config.MockModule(),
				fx.Supply(wmdef.NewParams()),
			))

			s := newWorkloadmetaObject(deps)

			s.handleEvents(test.preEvents)

			ch := s.Subscribe(dummySubscriber, wmdef.NormalPriority, nil)
			doneCh := make(chan struct{})

			var actualEventsReceived []wmdef.EventBundle
			go func() {
				for bundle := range ch {
					bundle.Acknowledge()

					// nil the bundle's Ch so we can deep-equal just the events
					// later
					bundle.Ch = nil

					actualEventsReceived = append(actualEventsReceived, bundle)
				}

				close(doneCh)
			}()

			s.Reset(test.newEntities, fooSource)

			// Force handling of events generated by the reset
			if len(s.eventCh) > 0 {
				s.handleEvents(<-s.eventCh)
			}

			s.Unsubscribe(ch)

			<-doneCh

			tassert.Equal(t, test.expectedEventsReceived, actualEventsReceived)
		})
	}
}

func TestNoDataRace(t *testing.T) { //nolint:revive // TODO fix revive unused-parameter
	// This test ensures that no race conditions are encountered when the "--race" flag is passed
	// to the test process and an entity is accessed in a different thread than the one handling events
	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	s := newWorkloadmetaObject(deps)

	container := &wmdef.Container{
		EntityID: wmdef.EntityID{
			Kind: wmdef.KindContainer,
			ID:   "123",
		},
	}

	go func() {
		_, _ = s.GetContainer("456")
	}()

	s.handleEvents([]wmdef.CollectorEvent{
		{
			Type:   wmdef.EventTypeSet,
			Source: fooSource,
			Entity: container,
		},
	})
}

func TestPushEvents(t *testing.T) {

	deps := fxutil.Test[dependencies](t, fx.Options(
		logimpl.MockModule(),
		config.MockModule(),
		fx.Supply(wmdef.NewParams()),
	))

	wlm := newWorkloadmetaObject(deps)

	mockSource := wmdef.Source("mockSource")

	tests := []struct {
		name        string
		events      []wmdef.Event
		source      wmdef.Source
		expectError bool
	}{
		{
			name:        "empty push events slice",
			events:      []wmdef.Event{},
			source:      mockSource,
			expectError: false,
		},
		{
			name: "push events with valid types",
			events: []wmdef.Event{
				{
					Type: wmdef.EventTypeSet,
				},
				{
					Type: wmdef.EventTypeUnset,
				},
				{
					Type: wmdef.EventTypeSet,
				},
			},
			source:      mockSource,
			expectError: false,
		},
		{
			name: "push events with invalid types",
			events: []wmdef.Event{
				{
					Type: wmdef.EventTypeSet,
				},
				{
					Type: wmdef.EventTypeAll,
				},
			},
			source:      mockSource,
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := wlm.Push(mockSource, test.events...)

			if test.expectError {
				tassert.Error(t, err, "Expected Push operation to fail and return error")
			} else {
				tassert.NoError(t, err, "Expected Push operation to succeed and return nil")
			}

		})
	}
}
