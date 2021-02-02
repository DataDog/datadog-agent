// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubelet

package kubelet

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	k "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/stretchr/testify/mock"
)

type kubeUtilMock struct {
	k.KubeUtilInterface
	mock.Mock
}

func (m *kubeUtilMock) GetNodename() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestHostnameProvider(t *testing.T) {
	mockConfig := config.Mock()

	ku := &kubeUtilMock{}

	ku.On("GetNodename").Return("node-name", nil)

	defer ku.AssertExpectations(t)

	kubeUtilGet = func() (k.KubeUtilInterface, error) {
		return ku, nil
	}

	hostName, err := HostnameProvider()
	assert.NoError(t, err)
	assert.Equal(t, "node-name", hostName)

	var testClusterName = "laika"
	mockConfig.Set("cluster_name", testClusterName)
	clustername.ResetClusterName() // reset state as clustername was already read

	// defer a reset of the state so that future hostname fetches are not impacted
	defer mockConfig.Set("cluster_name", "")
	defer clustername.ResetClusterName()

	hostName, err = HostnameProvider()
	assert.NoError(t, err)
	assert.Equal(t, "node-name-laika", hostName)
}
