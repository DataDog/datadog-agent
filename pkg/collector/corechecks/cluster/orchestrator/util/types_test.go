// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build orchestrator && test

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetResourceType(t *testing.T) {
	t.Run("deployment with constant values", func(t *testing.T) {
		result := GetResourceType(DeploymentName, DeploymentVersion)
		assert.Equal(t, "deployments.apps", result)
	})

	t.Run("pod with constant values", func(t *testing.T) {
		result := GetResourceType(PodName, PodVersion)
		assert.Equal(t, "pods", result)
	})

	t.Run("ingress with constant values", func(t *testing.T) {
		result := GetResourceType(IngressName, IngressVersion)
		assert.Equal(t, "ingresses.networking.k8s.io", result)
	})
}
