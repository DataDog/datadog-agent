// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type kubeUtilMock struct {
	k.KubeUtilInterface
	mock.Mock
}

func (m *kubeUtilMock) GetNodename(_ context.Context) (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestHostnameProvider(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	ctx := context.Background()
	mockConfig := configmock.New(t)

	ku := &kubeUtilMock{}

	ku.On("GetNodename").Return("node-name", nil)

	defer ku.AssertExpectations(t)

	kubeUtilGet = func() (k.KubeUtilInterface, error) {
		return ku, nil
	}

	hostName, err := GetHostname(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "node-name", hostName)

	testClusterName := "laika"
	mockConfig.SetInTest("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.SetInTest("cluster_name", "")
	defer clustername.ResetClusterName()

	hostName, err = GetHostname(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "node-name-laika", hostName)
}

func TestHostnameProviderWillBeValid(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	ctx := context.Background()
	mockConfig := configmock.New(t)

	ku := &kubeUtilMock{}

	ku.On("GetNodename").Return("node-name", nil)

	defer ku.AssertExpectations(t)

	kubeUtilGet = func() (k.KubeUtilInterface, error) {
		return ku, nil
	}

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.SetInTest("cluster_name", "")
	defer clustername.ResetClusterName()

	testClusterName := "laika_willbe_valid"
	mockConfig.SetInTest("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	hostName, err := GetHostname(ctx)
	assert.NoError(t, err)
	// In this case we make the clustername RFC1123 compliant
	assert.Equal(t, "node-name-laika-willbe-valid", hostName)
}

func TestHostnameProviderInvalid(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	ctx := context.Background()
	mockConfig := configmock.New(t)

	ku := &kubeUtilMock{}

	ku.On("GetNodename").Return("node-name", nil)

	defer ku.AssertExpectations(t)

	kubeUtilGet = func() (k.KubeUtilInterface, error) {
		return ku, nil
	}

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.SetInTest("cluster_name", "")
	defer clustername.ResetClusterName()

	testClusterName := "laika_invalid_"
	mockConfig.SetInTest("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	hostName, err := GetHostname(ctx)
	assert.NoError(t, err)
	// We won't use the clustername if its invalid RFC, we log an error and continue without the clustername and only hostname
	assert.Equal(t, "node-name", hostName)
}

func Test_makeClusterNameRFC1123Compliant(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		want        string
	}{
		{
			name:        "valid clustername",
			clusterName: "cluster-name",
			want:        "cluster-name",
		},
		{
			name:        "invalid clustername underscore",
			clusterName: "cluster_name",
			want:        "cluster-name",
		},
		{
			name:        "invalid clustername underscore at the end and middle",
			clusterName: "cluster_name_",
			want:        "cluster-name-",
		},
		{
			name:        "laika_invalid becomes laika-invalid",
			clusterName: "laika_invalid",
			want:        "laika-invalid",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := makeClusterNameRFC1123Compliant(tt.clusterName); got != tt.want {
				t.Errorf("makeClusterNameRFC1123Compliant() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHostnameProviderOTelStandalone verifies that otel-agent in DDOT
// standalone mode takes the node name from the environment, falling back to
// the kubelet only when it may use it, and the cluster name from
// OTEL_RESOURCE_ATTRIBUTES, and that other flavors keep using the kubelet.
func TestHostnameProviderOTelStandalone(t *testing.T) {
	tests := []struct {
		name                string
		flavor              string
		useKubeletCollector bool
		env                 map[string]string
		// kubeletNodename is what the kubelet reports. An empty one fails the
		// test if the kubelet is queried.
		kubeletNodename string
		want            string
		wantErr         bool
	}{
		{
			name:   "node name from the environment, cluster name from the resource attributes",
			flavor: flavor.OTelAgent,
			env: map[string]string{
				"OTEL_K8S_NODE_NAME":       "k3d-demo-server-0",
				"OTEL_RESOURCE_ATTRIBUTES": "k8s.node.name=k3d-demo-server-0,k8s.cluster.name=demo",
			},
			want: "k3d-demo-server-0-demo",
		},
		{
			name:   "node name from the environment, no cluster name",
			flavor: flavor.OTelAgent,
			env:    map[string]string{"K8S_NODE_NAME": "env-node"},
			want:   "env-node",
		},
		{
			name:                "environment preferred to the kubelet",
			flavor:              flavor.OTelAgent,
			useKubeletCollector: true,
			env:                 map[string]string{"K8S_NODE_NAME": "env-node"},
			want:                "env-node",
		},
		{
			name:                "falls back to the kubelet",
			flavor:              flavor.OTelAgent,
			useKubeletCollector: true,
			kubeletNodename:     "kubelet-node",
			want:                "kubelet-node",
		},
		{
			name:    "no kubelet fallback with the nodefilter collector",
			flavor:  flavor.OTelAgent,
			wantErr: true,
		},
		{
			name:    "invalid node name, no kubelet fallback with the nodefilter collector",
			flavor:  flavor.OTelAgent,
			env:     map[string]string{"K8S_NODE_NAME": "Not a node name"},
			wantErr: true,
		},
		{
			name:   "other flavors use the kubelet",
			flavor: flavor.DefaultAgent,
			env: map[string]string{
				"K8S_NODE_NAME":            "env-node",
				"OTEL_RESOURCE_ATTRIBUTES": "k8s.cluster.name=demo",
			},
			kubeletNodename: "kubelet-node",
			want:            "kubelet-node",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env.SetFeatures(t, env.Kubernetes)
			flavor.SetTestFlavor(t, tt.flavor)
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("cloud_provider_metadata", []string{})
			mockConfig.SetInTest("otel_standalone", true)
			mockConfig.SetInTest("otelcollector.standalone.use_kubelet_collector", tt.useKubeletCollector)
			for _, name := range []string{"K8S_NODE_NAME", "DD_KUBERNETES_KUBELET_NODENAME", "OTEL_K8S_NODE_NAME", "OTEL_RESOURCE_ATTRIBUTES"} {
				t.Setenv(name, tt.env[name])
			}
			clustername.ResetClusterName()
			t.Cleanup(clustername.ResetClusterName)

			previousKubeUtilGet := kubeUtilGet
			t.Cleanup(func() { kubeUtilGet = previousKubeUtilGet })
			kubeUtilGet = func() (k.KubeUtilInterface, error) {
				if tt.kubeletNodename == "" {
					t.Error("the kubelet was queried")
					return nil, errors.New("the kubelet must not be queried")
				}
				ku := &kubeUtilMock{}
				ku.On("GetNodename").Return(tt.kubeletNodename, nil)
				return ku, nil
			}

			hostName, err := GetHostname(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, hostName)
		})
	}
}
