// +build containerd

package containers

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/containerd/cgroups"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/types"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/typeurl"
	types2 "github.com/gogo/protobuf/types"
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

// Name is from the Image insterface
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
			"all functionning",
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
				//mockTask: func() (containerd.Task, error) {
				//	return nil, nil
				//},
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
			require.Equal(t, len(test.expected), len(list))
			require.True(t, reflect.DeepEqual(test.expected, list))
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
			"fully functionnal",
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
						Data: &types2.Any{
							TypeUrl: typeUrl,
							Value:   jsonValue,
						},
					}
					return metric, nil
				},
			}
			taskFacked := containerd.Task(te)
			m, e := convertTasktoMetrics(taskFacked, context.Background())
			require.Equal(t, test.expected, m)
			if e != nil {
				require.Equal(t, e.Error(), test.error)
			}
		})
	}
}
