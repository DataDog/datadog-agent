// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build containerd

package containerd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

type mockedContainerdClient struct {
	ContainerdItf
	mockNamespaces func(ctx context.Context) ([]string, error)
}

func (m *mockedContainerdClient) Namespaces(ctx context.Context) ([]string, error) {
	return m.mockNamespaces(ctx)
}

func TestNamespacesToWatch(t *testing.T) {
	tests := []struct {
		name                   string
		containerdNamespaceVal string
		client                 mockedContainerdClient
		expectedNamespaces     []string
		expectsError           bool
	}{
		{
			name:                   "containerd_namespace set",
			containerdNamespaceVal: "some_namespace",
			expectedNamespaces:     []string{"some_namespace"},
			expectsError:           false,
		},
		{
			name:                   "containerd_namespace not set",
			containerdNamespaceVal: "",
			client: mockedContainerdClient{mockNamespaces: func(ctx context.Context) ([]string, error) {
				return []string{"namespace_1", "namespace_2"}, nil
			}},
			expectedNamespaces: []string{"namespace_1", "namespace_2"},
			expectsError:       false,
		},
		{
			name: "error when getting namespaces",
			client: mockedContainerdClient{mockNamespaces: func(ctx context.Context) ([]string, error) {
				return nil, errors.New("some error")
			}},
			containerdNamespaceVal: "",
			expectsError:           true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Datadog.Set("containerd_namespace", test.containerdNamespaceVal)
			namespaces, err := NamespacesToWatch(context.TODO(), &test.client)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedNamespaces, namespaces)
			}
		})
	}
}
