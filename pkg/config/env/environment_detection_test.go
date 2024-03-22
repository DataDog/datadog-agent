// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExcludeFeatures(t *testing.T) {
	assert := assert.New(t)

	features := FeatureMap{
		CloudFoundry:             struct{}{},
		Containerd:               struct{}{},
		Cri:                      struct{}{},
		Docker:                   struct{}{},
		KubeOrchestratorExplorer: struct{}{},
		ECSOrchestratorExplorer:  struct{}{},
		Kubernetes:               struct{}{},
		ECSFargate:               struct{}{},
		EKSFargate:               struct{}{},
	}

	excludedFeatures := []string{
		"cri",
		"name:cri",
		"name:docker",
		"name:^.*fargate",
	}

	excludeFeatures(features, excludedFeatures)
	assert.Equal(features, FeatureMap{
		CloudFoundry:             struct{}{},
		Containerd:               struct{}{},
		KubeOrchestratorExplorer: struct{}{},
		ECSOrchestratorExplorer:  struct{}{},
		Kubernetes:               struct{}{},
	})
}
