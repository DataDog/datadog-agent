// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"testing"
)

func TestInstanceId(t *testing.T) {
	nodeSpecProviderId := "aws:///us-east-1b/i-024b28584ed2e6321"

	instanceId := extractInstanceIdFromProviderId(v1.NodeSpec{ProviderID: nodeSpecProviderId})
	assert.Equal(t, "i-024b28584ed2e6321", instanceId)
}
