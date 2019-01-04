// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build containerd

package containers

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	containersutil "github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
	"github.com/docker/docker/pkg/testutil/assert"
	prototypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockContainer struct {
	containerd.Container
	mockTask   func() (containerd.Task, error)
	mockImage  func() (containerd.Image, error)
	mockLabels func() (map[string]string, error)
	mockInfo   func() (containers.Container, error)
}

// Task is from the containerd.Container interface
func (cs *mockContainer) Task(context.Context, cio.Attach) (containerd.Task, error) {
	return cs.mockTask()
}

// Image is from the containerd.Container interface
func (cs *mockContainer) Image(context.Context) (containerd.Image, error) {
	return cs.mockImage()
}

// Labels is from the containerd.Container interface
func (cs *mockContainer) Labels(context.Context) (map[string]string, error) {
	return cs.mockLabels()
}

// Info is from the containerd.Container interface
func (cs *mockContainer) Info(context.Context) (containers.Container, error) {
	return cs.mockInfo()
}

type mockTaskStruct struct {
	containerd.Task
	mockMectric func(ctx context.Context) (*types.Metric, error)
}

// Metrics is from the containerd.Task interface
func (t *mockTaskStruct) Metrics(ctx context.Context) (*types.Metric, error) {
	return t.mockMectric(ctx)
}

type mockImage struct {
	imageName string
	containerd.Image
}

// Name is from the Image interface
func (i *mockImage) Name() string {
	return i.imageName
}

// TestCollectTags checks the collectTags method
func TestCollectTags(t *testing.T) {
	img := &mockImage{}
	tests := []struct {
		name      string
		labels    map[string]string
		imageName string
		runtime   string
		expected  []string
		err       error
	}{
		{
			"all functioning",
			map[string]string{"foo": "bar"},
			"redis",
			"containerd",
			[]string{"runtime:containerd", "image:redis", "foo:bar"},
			nil,
		}, {
			"missing labels",
			map[string]string{},
			"imagename",
			"containerd",
			[]string{"runtime:containerd", "image:imagename"},
			nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cs := &mockContainer{
				mockImage: func() (containerd.Image, error) {
					img.imageName = test.imageName
					return containerd.Image(img), nil
				},
				mockLabels: func() (map[string]string, error) {
					return test.labels, nil
				},
				mockInfo: func() (containers.Container, error) {
					ctn := containers.Container{
						Runtime: containers.RuntimeInfo{
							Name: test.runtime,
						},
					}
					return ctn, nil
				},
			}
			ctn := containerd.Container(cs)
			list, err := collectTags(ctn, context.Background())
			if err != nil {
				require.Error(t, test.err, err)
			}
			sort.Strings(list)
			sort.Strings(test.expected)
			require.EqualValues(t, test.expected, list)
		})
	}
}

// TestComputeEvents checks the conversion of Containerd events to Datadog events
func TestComputeEvents(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance: &ContainerdConfig{
			Tags: []string{"test"},
		},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	mocked := mocksender.NewMockSender(containerdCheck.ID())
	var err error
	containerdCheck.filters, err = containersutil.GetSharedFilter()
	require.NoError(t, err)

	tests := []struct {
		name          string
		events        []containerdEvent
		expectedTitle string
		expectedTags  []string
		numberEvents  int
	}{
		{
			name:          "No events",
			events:        []containerdEvent{},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "Events on wrong type",
			events: []containerdEvent{{
				Topic: "/containers/delete/extra",
			}, {
				Topic: "containers/delete",
			},
			},
			expectedTitle: "",
			numberEvents:  0,
		},
		{
			name: "High cardinality Events with one invalid",
			events: []containerdEvent{{
				Topic:     "/containers/delete",
				Timestamp: time.Now(),
				Extra:     map[string]string{"foo": "bar"},
				Message:   "Container xxx deleted",
				ID:        "xxx",
			}, {
				Topic: "containers/delete",
			},
			},
			expectedTitle: "Event on containers from Containerd",
			expectedTags:  []string{"foo:bar", "test"},
			numberEvents:  1,
		},
		{
			name: "Low cardinality Event",
			events: []containerdEvent{{
				Topic:     "/images/update",
				Timestamp: time.Now(),
				Extra:     map[string]string{"foo": "baz"},
				Message:   "Image yyy updated",
				ID:        "yyy",
			},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  []string{"foo:baz", "test"},
			numberEvents:  1,
		},
		{
			name: "Filtered event",
			events: []containerdEvent{{
				Topic:     "/images/create",
				Timestamp: time.Now(),
				Extra:     map[string]string{},
				Message:   "Image kubernetes/pause created",
				ID:        "kubernetes/pause",
			},
			},
			expectedTitle: "Event on images from Containerd",
			expectedTags:  []string{},
			numberEvents:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeEvents(test.events, mocked, containerdCheck.instance.Tags, containerdCheck.filters)
			mocked.On("Event", mock.AnythingOfType("metrics.Event"))
			if len(mocked.Calls) > 0 {
				res := (mocked.Calls[0].Arguments.Get(0)).(metrics.Event)
				assert.Contains(t, res.Title, test.expectedTitle)
				assert.EqualStringSlice(t, res.Tags, test.expectedTags)
			}
			mocked.AssertNumberOfCalls(t, "Event", test.numberEvents)
			mocked.ResetCalls()
		})
	}
}

// TestConvertTaskToMetrics checks the convertTasktoMetrics
func TestConvertTaskToMetrics(t *testing.T) {
	typeurl.Register(&cgroups.Metrics{}, "io.containerd.cgroups.v1.Metrics") // Need to register the type to be used in UnmarshalAny later on.

	tests := []struct {
		name     string
		typeUrl  string
		values   cgroups.Metrics
		error    string
		expected *cgroups.Metrics
	}{
		{
			"unregistered type",
			"io.containerd.cgroups.v1.Doge",
			cgroups.Metrics{},
			"type with url io.containerd.cgroups.v1.Doge: not found",
			nil,
		},
		{
			"missing values",
			"io.containerd.cgroups.v1.Metrics",
			cgroups.Metrics{},
			"",
			&cgroups.Metrics{},
		},
		{
			"fully functional",
			"io.containerd.cgroups.v1.Metrics",
			cgroups.Metrics{Memory: &cgroups.MemoryStat{Cache: 100}},
			"",
			&cgroups.Metrics{
				Memory: &cgroups.MemoryStat{
					Cache: 100,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			te := &mockTaskStruct{
				mockMectric: func(ctx context.Context) (*types.Metric, error) {
					typeUrl := test.typeUrl
					jsonValue, _ := json.Marshal(test.values)
					metric := &types.Metric{
						Data: &prototypes.Any{
							TypeUrl: typeUrl,
							Value:   jsonValue,
						},
					}
					return metric, nil
				},
			}
			taskFaked := containerd.Task(te)
			m, e := convertTasktoMetrics(taskFaked, context.Background())
			require.Equal(t, test.expected, m)
			if e != nil {
				require.Equal(t, e.Error(), test.error)
			}
		})
	}
}

// TestisExcluded tests the filtering of containers in the compute metrics method
func TestIsExcluded(t *testing.T) {
	containerdCheck := &ContainerdCheck{
		instance:  &ContainerdConfig{},
		CheckBase: corechecks.NewCheckBase("containerd"),
	}
	var err error
	// GetShareFilter gives us the OOB exclusion of pause container images from most supported platforms
	containerdCheck.filters, err = containersutil.GetSharedFilter()
	require.NoError(t, err)
	img := &mockImage{}
	c := &mockContainer{
		mockImage: func() (containerd.Image, error) {
			img.imageName = "kubernetes/pause"
			return containerd.Image(img), nil
		},
	}
	// kubernetes/pause is excluded
	isEc := isExcluded(c, context.Background(), containerdCheck.filters)
	require.True(t, isEc)

	c.mockImage = func() (containerd.Image, error) {
		img.imageName = "kubernetes/pawz"
		return containerd.Image(img), nil
	}
	// kubernetes/pawz although not an available image (yet ?) is not ignored
	isEc = isExcluded(c, context.Background(), containerdCheck.filters)
	require.False(t, isEc)
}
