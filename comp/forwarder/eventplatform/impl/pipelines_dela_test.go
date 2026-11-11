// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// TestEventPlatformPipelinesAreEnrolledForDelegatedAuth pins the delegated-auth list-key
// registry against this package's pipeline descriptors: every pipeline's
// <prefix>additional_endpoints setting must be enrolled, so a new pipeline that forgets to
// register its prefix fails here instead of silently dropping DELA[...] directives.
func TestEventPlatformPipelinesAreEnrolledForDelegatedAuth(t *testing.T) {
	descs := getPassthroughPipelines()
	assert.NotEmpty(t, descs)

	for _, desc := range descs {
		key := desc.endpointsConfigPrefix + "additional_endpoints"
		assert.True(t, pkgconfigmodel.IsDelaListEndpointsKey(key),
			"pipeline %q (prefix %q) must have its additional_endpoints setting enrolled for delegated auth",
			desc.eventType, desc.endpointsConfigPrefix)
	}
}
