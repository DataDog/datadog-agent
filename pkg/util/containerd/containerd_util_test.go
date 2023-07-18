// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	v1 "github.com/containerd/cgroups/stats/v1"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/typeurl"
	prototypes "github.com/gogo/protobuf/types"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
)

type mockContainer struct {
	containerd.Container
	mockTask   func() (containerd.Task, error)
	mockImage  func() (containerd.Image, error)
	mockLabels func() (map[string]string, error)
	mockInfo   func() (containers.Container, error)
	mockSpec   func() (*oci.Spec, error)
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
func (cs *mockContainer) Info(context.Context, ...containerd.InfoOpts) (containers.Container, error) {
	return cs.mockInfo()
}

func (cs *mockContainer) Spec(context.Context) (*oci.Spec, error) {
	return cs.mockSpec()
}

type mockTaskStruct struct {
	containerd.Task
	mockMectric func(ctx context.Context) (*types.Metric, error)
	mockStatus  func(ctx context.Context) (containerd.Status, error)
}

// Metrics is from the containerd.Task interface
func (t *mockTaskStruct) Metrics(ctx context.Context) (*types.Metric, error) {
	return t.mockMectric(ctx)
}

func (t *mockTaskStruct) Status(ctx context.Context) (containerd.Status, error) {
	return t.mockStatus(ctx)
}

type mockImage struct {
	size int64
	containerd.Image
}

// Name is from the Image interface
func (i *mockImage) Size(ctx context.Context) (int64, error) {
	return i.size, nil
}

const TestNamespace = "default"

func TestEnvVars(t *testing.T) {
	tests := []struct {
		name           string
		specEnvs       []string
		filterFunc     func(string) bool
		expectedResult map[string]string
		expectsErr     bool
	}{
		{
			name:           "valid envs",
			specEnvs:       []string{"ENV1=val1", "ENV2=val2"},
			expectedResult: map[string]string{"ENV1": "val1", "ENV2": "val2"},
		},
		{
			name:     "valid envs",
			specEnvs: []string{"ENV1=val1", "ENV2=val2"},
			filterFunc: func(s string) bool {
				return s == "ENV1"
			},
			expectedResult: map[string]string{"ENV1": "val1"},
		},
		{
			name:       "wrong format",
			specEnvs:   []string{"ENV1/val1"},
			expectsErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := &oci.Spec{
				Process: &specs.Process{
					Env: test.specEnvs,
				},
			}

			envVars, err := EnvVarsFromSpec(spec, test.filterFunc)

			if test.expectsErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expectedResult, envVars)
			}
		})
	}
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
	c, err := mockUtil.Info(TestNamespace, ctn)
	require.NoError(t, err)
	require.Equal(t, "foo", c.Image)
}

func TestImageOfContainer(t *testing.T) {
	mockUtil := ContainerdUtil{}

	image := &mockImage{
		size: 5,
	}

	container := &mockContainer{
		mockImage: func() (containerd.Image, error) {
			return image, nil
		},
	}

	resultImage, err := mockUtil.ImageOfContainer(TestNamespace, container)
	require.NoError(t, err)
	require.Equal(t, resultImage, image)
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
	c, err := mockUtil.ImageSize(TestNamespace, ctn)
	require.NoError(t, err)
	require.Equal(t, int64(12), c)
}

func TestTaskMetrics(t *testing.T) {
	mockUtil := ContainerdUtil{}
	typeurl.Register(&v1.Metrics{}, "io.containerd.cgroups.v1.Metrics") // Need to register the type to be used in UnmarshalAny later on.

	tests := []struct {
		name            string
		typeURL         string
		values          v1.Metrics
		error           string
		taskMetricError error
		expected        *v1.Metrics
	}{
		{
			"fully functional",
			"io.containerd.cgroups.v1.Metrics",
			v1.Metrics{Memory: &v1.MemoryStat{Cache: 100}},
			"",
			nil,
			&v1.Metrics{
				Memory: &v1.MemoryStat{
					Cache: 100,
				},
			},
		},
		{
			"type not registered",
			"io.containerd.cgroups.v1.Doge",
			v1.Metrics{Memory: &v1.MemoryStat{Cache: 10}},
			"type with url io.containerd.cgroups.v1.Doge: not found",
			nil,
			&v1.Metrics{
				Memory: &v1.MemoryStat{
					Cache: 10,
				},
			},
		},
		{
			"task does not exist",
			"io.containerd.cgroups.v1.Metric",
			v1.Metrics{},
			"",
			fmt.Errorf("no running task found"),
			&v1.Metrics{},
		},
		{
			"task does not exist",
			"io.containerd.cgroups.v1.Metric",
			v1.Metrics{},
			"",
			fmt.Errorf("no metrics received"),
			&v1.Metrics{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cton := makeCtn(test.values, test.typeURL, test.taskMetricError)

			m, e := mockUtil.TaskMetrics(TestNamespace, cton)
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
			}
			require.Equal(t, test.expected, metricAny.(*v1.Metrics))
		})
	}
}

func TestStatus(t *testing.T) {
	mockUtil := ContainerdUtil{}

	status := containerd.Running

	task := mockTaskStruct{
		mockStatus: func(ctx context.Context) (containerd.Status, error) {
			return containerd.Status{
				Status: status,
			}, nil
		},
	}

	container := &mockContainer{
		mockTask: func() (containerd.Task, error) {
			return &task, nil
		},
	}

	resultStatus, err := mockUtil.Status(TestNamespace, container)
	require.NoError(t, err)
	require.Equal(t, resultStatus, status)
}

func TestIsSandbox(t *testing.T) {
	mockUtil := ContainerdUtil{}

	withSandboxLabel := &mockContainer{
		mockSpec: func() (*oci.Spec, error) {
			return &oci.Spec{Annotations: map[string]string{}}, nil
		},
		mockLabels: func() (map[string]string, error) {
			return map[string]string{"io.cri-containerd.kind": "sandbox"}, nil
		},
	}

	isSandbox, err := mockUtil.IsSandbox(TestNamespace, withSandboxLabel)
	require.NoError(t, err)
	require.True(t, isSandbox)

	notSandbox := &mockContainer{
		mockSpec: func() (*oci.Spec, error) {
			return &oci.Spec{Annotations: map[string]string{"annotation_key": "annotation_val"}}, nil
		},
		mockLabels: func() (map[string]string, error) {
			return map[string]string{"label_key": "label_val"}, nil
		},
	}

	isSandbox, err = mockUtil.IsSandbox(TestNamespace, notSandbox)
	require.NoError(t, err)
	require.False(t, isSandbox)
}

func makeCtn(value v1.Metrics, typeURL string, taskMetricsError error) containerd.Container {
	taskStruct := &mockTaskStruct{
		mockMectric: func(ctx context.Context) (*types.Metric, error) {
			typeURL := typeURL
			jsonValue, _ := json.Marshal(value)
			metric := &types.Metric{
				Data: &prototypes.Any{
					TypeUrl: typeURL,
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
