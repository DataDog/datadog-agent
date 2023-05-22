// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"

	"github.com/stretchr/testify/assert"
)

func TestNamespacesToWatch(t *testing.T) {
	tests := []struct {
		name                   string
		containerdNamespaceVal string
		excludeNamespaceVal    string
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
			client: &fake.MockedContainerdClient{MockNamespaces: func(context.Context) ([]string, error) {
				return []string{"namespace_1", "namespace_2"}, nil
			}},
			expectedNamespaces: []string{"namespace_1", "namespace_2"},
			expectsError:       false,
		},
		{
			name:                   "containerd_namespace not set, containerd_exclude_namespaces set",
			containerdNamespaceVal: "",
			excludeNamespaceVal:    "namespace_2",
			client: &fake.MockedContainerdClient{MockNamespaces: func(context.Context) ([]string, error) {
				return []string{"namespace_1", "namespace_2"}, nil
			}},
			expectedNamespaces: []string{"namespace_1"},
			expectsError:       false,
		},
		{
			name: "error when getting namespaces",
			client: &fake.MockedContainerdClient{MockNamespaces: func(context.Context) ([]string, error) {
				return nil, errors.New("some error")
			}},
			containerdNamespaceVal: "",
			expectsError:           true,
		},
	}

	originalContainerdNamespacesOpt := config.Datadog.GetStringSlice("containerd_namespaces")
	originalExcludeNamespacesOpt := config.Datadog.GetStringSlice("containerd_exclude_namespaces")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Datadog.Set("containerd_namespaces", test.containerdNamespaceVal)
			defer config.Datadog.Set("containerd_namespaces", originalContainerdNamespacesOpt)

			config.Datadog.Set("containerd_exclude_namespaces", test.excludeNamespaceVal)
			defer config.Datadog.Set("containerd_exclude_namespaces", originalExcludeNamespacesOpt)

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
		name                   string
		containerdNamespaceVal string
		excludeNamespaceVal    string
		inputFilters           []string
		expectedFilters        []string
	}{
		{
			name:                   "watch all namespaces",
			containerdNamespaceVal: "",
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
			name:                   "watch all namespaces, exclude some",
			containerdNamespaceVal: "",
			excludeNamespaceVal:    "exclude1 exclude2",
			inputFilters: []string{
				`topic==/containers/create`,
				`topic==/containers/delete`,
			},
			expectedFilters: []string{
				`topic==/containers/create,namespace!="exclude1"`,
				`topic==/containers/delete,namespace!="exclude1"`,
				`topic==/containers/create,namespace!="exclude2"`,
				`topic==/containers/delete,namespace!="exclude2"`,
			},
		},
		{
			name:                   "watch one namespace",
			containerdNamespaceVal: "ns1",
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
			name:                   "watch several namespaces, but not all",
			containerdNamespaceVal: "ns1 ns2",
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

	originalContainerdNamespacesOpt := config.Datadog.GetStringSlice("containerd_namespaces")
	originalExcludeNamespacesOpt := config.Datadog.GetStringSlice("containerd_exclude_namespaces")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config.Datadog.Set("containerd_namespaces", test.containerdNamespaceVal)
			defer config.Datadog.Set("containerd_namespaces", originalContainerdNamespacesOpt)

			config.Datadog.Set("containerd_exclude_namespaces", test.excludeNamespaceVal)
			defer config.Datadog.Set("containerd_exclude_namespaces", originalExcludeNamespacesOpt)

			result := FiltersWithNamespaces(test.inputFilters)
			assert.ElementsMatch(t, test.expectedFilters, result)
		})
	}
}
