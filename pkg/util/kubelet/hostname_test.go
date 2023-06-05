// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

type kubeUtilMock struct {
	k.KubeUtilInterface
	mock.Mock
}

func (m *kubeUtilMock) GetNodename(ctx context.Context) (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestHostnameProvider(t *testing.T) {
	config.SetFeatures(t, config.Kubernetes)

	ctx := context.Background()
	mockConfig := config.Mock(t)

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
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", "")
	defer clustername.ResetClusterName()

	hostName, err = GetHostname(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "node-name-laika", hostName)
}

func TestHostnameProviderInvalid(t *testing.T) {
	config.SetFeatures(t, config.Kubernetes)

	ctx := context.Background()
	mockConfig := config.Mock(t)

	ku := &kubeUtilMock{}

	ku.On("GetNodename").Return("node-name", nil)

	defer ku.AssertExpectations(t)

	kubeUtilGet = func() (k.KubeUtilInterface, error) {
		return ku, nil
	}

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", "")
	defer clustername.ResetClusterName()

	testClusterName := "laika_invalid"
	mockConfig.Set("cluster_name", testClusterName)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := makeClusterNameRFC1123Compliant(tt.clusterName); got != tt.want {
				t.Errorf("makeClusterNameRFC1123Compliant() = %v, want %v", got, tt.want)
			}
		})
	}
}
