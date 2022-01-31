// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

func TestNamespacesToWatch(t *testing.T) {
	tests := []struct {
		name                   string
		containerdNamespaceVal string
		client                 ContainerdItf
		expectedNamespaces     []string
		expectsError           bool
	}{
		{
			name:                   "containerd_namespace set with one namespace",
			containerdNamespaceVal: "some_namespace",
			expectedNamespaces:     []string{"some_namespace"},
			expectsError:           false,
		},
		{
			name:                   "containerd_namespace set with multiple namespaces",
			containerdNamespaceVal: "ns1 ns2 ns3",
			expectedNamespaces:     []string{"ns1", "ns2", "ns3"},
			expectsError:           false,
		},
		{
			name:                   "containerd_namespace not set",
			containerdNamespaceVal: "",
			client: &fake.MockedContainerdClient{MockNamespaces: func(ctx context.Context) ([]string, error) {
				return []string{"namespace_1", "namespace_2"}, nil
			}},
			expectedNamespaces: []string{"namespace_1", "namespace_2"},
			expectsError:       false,
		},
		{
			name: "error when getting namespaces",
			client: &fake.MockedContainerdClient{MockNamespaces: func(ctx context.Context) ([]string, error) {
				return nil, errors.New("some error")
			}},
			containerdNamespaceVal: "",
			expectsError:           true,
		},
	}

	originalContainerdNamespaceOpt := config.Datadog.GetString("containerd_namespace")
	defer config.Datadog.Set("containerd_namespace", originalContainerdNamespaceOpt)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Datadog.Set("containerd_namespace", test.containerdNamespaceVal)
			namespaces, err := NamespacesToWatch(context.TODO(), test.client)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedNamespaces, namespaces)
			}
		})
	}
}

func TestFiltersWithNamespaces(t *testing.T) {
	tests := []struct {
		name                         string
		containerdNamespaceConfigOpt string
		inputFilters                 []string
		expectedFilters              []string
	}{
		{
			name:                         "watch all namespaces",
			containerdNamespaceConfigOpt: "",
			inputFilters: []string{
				`topic==/containers/create`,
				`topic==/containers/delete`,
			},
			expectedFilters: []string{
				`topic==/containers/create`,
				`topic==/containers/delete`,
			},
		},
		{
			name:                         "watch one namespace",
			containerdNamespaceConfigOpt: "ns1",
			inputFilters: []string{
				`topic=="/containers/create"`,
				`topic=="/containers/delete"`,
			},
			expectedFilters: []string{
				`topic=="/containers/create",namespace=="ns1"`,
				`topic=="/containers/delete",namespace=="ns1"`,
			},
		},
		{
			name:                         "watch several namespaces, but not all",
			containerdNamespaceConfigOpt: "ns1 ns2",
			inputFilters: []string{
				`topic=="/containers/create"`,
				`topic=="/containers/delete"`,
			},
			expectedFilters: []string{
				`topic=="/containers/create",namespace=="ns1"`,
				`topic=="/containers/delete",namespace=="ns1"`,
				`topic=="/containers/create",namespace=="ns2"`,
				`topic=="/containers/delete",namespace=="ns2"`,
			},
		},
	}

	originalContainerdNamespaceOpt := config.Datadog.GetString("containerd_namespace")
	defer config.Datadog.Set("containerd_namespace", originalContainerdNamespaceOpt)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Datadog.Set("containerd_namespace", test.containerdNamespaceConfigOpt)
			result := FiltersWithNamespaces(test.inputFilters)
			assert.ElementsMatch(t, test.expectedFilters, result)
		})
	}
}
