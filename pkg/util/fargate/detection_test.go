// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fargate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetEKSFargateNodenameWithNodename(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("kubernetes_kubelet_nodename", "test-node-name")

	nodename, err := GetEKSFargateNodename()
	assert.NoError(t, err)
	assert.Equal(t, "test-node-name", nodename)
}

func TestGetEKSFargateNodenameWithoutNodename(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("kubernetes_kubelet_nodename", "")

	_, err := GetEKSFargateNodename()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubernetes_kubelet_nodename is not defined")
}
