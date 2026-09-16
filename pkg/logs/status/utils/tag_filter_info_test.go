// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagFilterInfoAllScopes(t *testing.T) {
	info := NewTagFilterInfo(
		[]string{"kube_namespace:*"},
		[]string{"container_id:*", "kube_replica_set:*"},
		[]string{"kube_namespace:*"},
		[]string{"filename:*", "dirname:*"},
	)
	expected := []string{
		"global include: kube_namespace:*",
		"global exclude: container_id:*, kube_replica_set:*",
		"source include: kube_namespace:*",
		"source exclude: filename:*, dirname:*",
	}
	assert.Equal(t, expected, info.Info())
}

// TestTagFilterInfoSkippedWhenUnconfigured pins the requirement that an unconfigured source
// shows no Tag Filters block at all: InfoRegistry skips providers that render nothing.
func TestTagFilterInfoSkippedWhenUnconfigured(t *testing.T) {
	reg := NewInfoRegistry()
	reg.Register(NewTagFilterInfo(nil, nil, nil, nil))

	rendered := reg.RenderedVerbose(true)
	assert.NotContains(t, rendered, "Tag Filters")
}
