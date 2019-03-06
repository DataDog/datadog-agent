// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
	prototypes "github.com/gogo/protobuf/types"
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
	size      int64
	containerd.Image
}

// Name is from the Image interface
func (i *mockImage) Size(ctx context.Context) (int64, error) {
	return i.size, nil
}

func TestInfo(t *testing.T) {
	mockUtil := ContainerdUtil{}
	cs := &mockContainer{
		mockInfo: func() (containers.Container, error) {
			ctn := containers.Container{
				Image: "foo",
			}
			return ctn, nil
		},
	}
	ctn := containerd.Container(cs)
	c, err := mockUtil.Info(ctn)
	require.NoError(t, err)
	require.Equal(t, "foo", c.Image)
}

func TestImageSize(t *testing.T) {
	mockUtil := ContainerdUtil{}

	cs := &mockContainer{
		mockImage: func() (containerd.Image, error) {
			return &mockImage{
				size: 12,
			}, nil
		},
	}
	ctn := containerd.Container(cs)
	c, err := mockUtil.ImageSize(ctn)
	require.NoError(t, err)
	require.Equal(t, int64(12), c)
}

func TestTaskMetrics(t *testing.T) {
	mockUtil := ContainerdUtil{}
	typeurl.Register(&cgroups.Metrics{}, "io.containerd.cgroups.v1.Metrics") // Need to register the type to be used in UnmarshalAny later on.

	tests := []struct {
		name            string
		typeUrl         string
		values          cgroups.Metrics
		error           string
		taskMetricError error
		expected        *cgroups.Metrics
	}{
		{
			"fully functional",
			"io.containerd.cgroups.v1.Metrics",
			cgroups.Metrics{Memory: &cgroups.MemoryStat{Cache: 100}},
			"",
			nil,
			&cgroups.Metrics{
				Memory: &cgroups.MemoryStat{
					Cache: 100,
				},
			},
		},
		{
			"type not registered",
			"io.containerd.cgroups.v1.Doge",
			cgroups.Metrics{Memory: &cgroups.MemoryStat{Cache: 10}},
			"type with url io.containerd.cgroups.v1.Doge: not found",
			nil,
			&cgroups.Metrics{
				Memory: &cgroups.MemoryStat{
					Cache: 10,
				},
			},
		},
		{
			"task does not exist",
			"io.containerd.cgroups.v1.Metric",
			cgroups.Metrics{},
			"",
			fmt.Errorf("no running task found"),
			&cgroups.Metrics{},
		},
		{
			"task does not exist",
			"io.containerd.cgroups.v1.Metric",
			cgroups.Metrics{},
			"",
			fmt.Errorf("no metrics received"),
			&cgroups.Metrics{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			ctn := makeCtn(test.values, test.typeUrl, test.taskMetricError)

			cton := containerd.Container(ctn)

			m, e := mockUtil.TaskMetrics(cton)
			if e != nil {
				require.Equal(t, e, test.taskMetricError)
				return
			}

			metricAny, err := typeurl.UnmarshalAny(&prototypes.Any{
				TypeUrl: m.Data.TypeUrl,
				Value:   m.Data.Value,
			})
			if err != nil {
				require.Equal(t, err.Error(), test.error)
				return
			} else {
				require.Equal(t, test.expected, metricAny.(*cgroups.Metrics))
			}
		})
	}
}

func makeCtn(value cgroups.Metrics, typeUrl string, taskMetricsError error) containerd.Container {
	taskStruct := &mockTaskStruct{
		mockMectric: func(ctx context.Context) (*types.Metric, error) {
			typeUrl := typeUrl
			jsonValue, _ := json.Marshal(value)
			metric := &types.Metric{
				Data: &prototypes.Any{
					TypeUrl: typeUrl,
					Value:   jsonValue,
				},
			}
			return metric, taskMetricsError
		},
	}

	ctn := &mockContainer{
		mockTask: func() (containerd.Task, error) {
			return taskStruct, taskMetricsError
		},
	}
	return ctn
}
