// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build containerd

package containers

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
	"github.com/docker/docker/pkg/testutil/assert"
	prototypes "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/StackVista/stackstate-agent/pkg/aggregator/mocksender"
	"github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/metrics"
	containersutil "github.com/StackVista/stackstate-agent/pkg/util/containers"
)

// TestCollectTags checks the collectTags method
func TestCollectTags(t *testing.T) {
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
			ctn := containers.Container{
				Image:   test.imageName,
				Labels:  test.labels,
				Runtime: containers.RuntimeInfo{Name: test.runtime},
			}
			list, err := collectTags(ctn)
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
		instance:  &ContainerdConfig{},
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
			expectedTags:  []string{"foo:bar"},
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
			expectedTags:  []string{"foo:baz"},
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
			expectedTags:  nil,
			numberEvents:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			computeEvents(test.events, mocked, containerdCheck.filters)
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
			typeUrl := test.typeUrl
			jsonValue, _ := json.Marshal(test.values)
			metric := &types.Metric{
				Data: &prototypes.Any{
					TypeUrl: typeUrl,
					Value:   jsonValue,
				},
			}
			m, e := convertTasktoMetrics(metric)
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
	c := containers.Container{
		Image: "kubernetes/pause",
	}
	// kubernetes/pause is excluded
	isEc := isExcluded(c, containerdCheck.filters)
	require.True(t, isEc)

	c = containers.Container{
		Image: "kubernetes/pawz",
	}
	// kubernetes/pawz although not an available image (yet ?) is not ignored
	isEc = isExcluded(c, containerdCheck.filters)
	require.False(t, isEc)
}
