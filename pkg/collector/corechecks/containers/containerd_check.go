// +build containerd

package containers

import (
	"testing"
	"context"
	"github.com/containerd/containerd"
	"fmt"
	containerd2 "github.com/DataDog/datadog-agent/pkg/util/containerd"
)

type mockContainerdUtil struct {
	mockConvertEvents func() containerd.EventService
}
func (m *mockContainerdUtil) GetEvents() containerd.EventService {
	m.mockConvertEvents()
	return nil
}

func (m *mockContainerdUtil) EnsureServing(ctx context.Context) error {return nil}
func (m *mockContainerdUtil) GetNamespaces(ctx context.Context) ([]string, error) {return nil, nil}
func (m *mockContainerdUtil) Containers(ctx context.Context) ([]containerd.Container, error) {return nil, nil}

func TestContainerdUtil_GetEvents(t *testing.T) {

	m := &mockContainerdUtil{
		mockConvertEvents: func() containerd.EventService {
			fmt.Printf("return events")
			return nil
		},
	}
	containerdCheck := ContainerdCheck{
		cu: containerd2.ContainerdItf(m),
	}
	containerdCheck.GetMetricStats()
	//tests := []struct {
	//	name string
	//}{
	//	// TODO: test cases
	//}
	//for _, test := range tests {
	//	t.Run(test.name, func(t *testing.T) {
	//
	//	})
	//}
}
