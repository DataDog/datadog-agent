// +build containerd

package containers

import (
	"testing"
	"context"
	"github.com/containerd/containerd"
	"fmt"
	containerd2 "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/containerd/containerd/cio"
	"github.com/stretchr/testify/require"
)

type mockContainerdUtil struct {
	mockConvertEvents func() containerd.EventService
	mockContainers    func() ([]containerd.Container, error)
}
func (m *mockContainerdUtil) GetEvents() containerd.EventService {
	m.mockConvertEvents()
	return nil
}
type ContainerS struct {
	containerd.Container
	mockTask func() (containerd.Task, error)
}
func (cs *ContainerS) Task(context.Context, cio.Attach) (containerd.Task, error){
	return cs.mockTask()
}
func (m *mockContainerdUtil) EnsureServing(ctx context.Context) error {return nil}
func (m *mockContainerdUtil) GetNamespaces(ctx context.Context) ([]string, error) {return []string{"k8s.io", "default"}, nil}
func (m *mockContainerdUtil) Containers(ctx context.Context) ([]containerd.Container, error) {return m.mockContainers()}

func TestContainerd_computeMetrics(t *testing.T) {

	m := &mockContainerdUtil{
		mockConvertEvents: func() containerd.EventService {
			fmt.Printf("return events")
			return nil
		},
	}
	containerdCheck := ContainerdCheck{
		cu: containerd2.ContainerdItf(m),
	}
	s := mocksender.NewMockSender(containerdCheck.ID())

	computeMetrics(s, context.Background(), containerdCheck.cu, nil)

	tests := []struct {
		name string
	}{
		// TODO: test cases
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

		})
	}
}

func TestConvertTaskToMetrics(t *testing.T) {
	cs := &ContainerS{
		mockTask: func() (containerd.Task, error) {
			fmt.Printf("test")
			return nil, nil
		},
	}

	tes := containerd.Container(cs)
	_, e := tes.Task(context.Background(), nil)

	require.Error(t, e)

	//m := &mockContainerdUtil{
	//	mockContainers: func() ([]containerd.Container, error) {
	//
	//	},
	//}
	//containerdCheck := ContainerdCheck{
	//	cu: containerd2.ContainerdItf(m),
	//}
	//s := mocksender.NewMockSender(containerdCheck.ID())

	//m.Containers(context.Background())

}
