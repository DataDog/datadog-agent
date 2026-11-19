// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package eksidentity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
)

func TestResolveDistributionNotDeterminedYet(t *testing.T) {
	// The real DCAGetDistribution without an API server, as when the Cluster Agent API is up
	// before WaitForAPIClient returns. Nothing is cached, so this is not "not EKS".
	state := stub(t, "eks", "orders", nil, "orders")
	detectDistribution = cloudprovider.DCAGetDistribution
	cache.Cache.Delete(cache.BuildAgentKey(constants.NodeKubeDistributionKey))
	t.Cleanup(func() { cache.Cache.Delete(cache.BuildAgentKey(constants.NodeKubeDistributionKey)) })

	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrNotDetermined)
	assert.NotErrorIs(t, err, ErrNotEKS)
	assert.Equal(t, 0, state.ec2Lookups)
}
