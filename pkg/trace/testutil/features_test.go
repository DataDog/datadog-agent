// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config/features"

	"github.com/stretchr/testify/assert"
)

func TestWithFeatures(t *testing.T) {
	assert.False(t, features.Has("unknown_feature"))
	undo := WithFeatures("unknown_feature,other")
	assert.True(t, features.Has("unknown_feature"))
	assert.True(t, features.Has("other"))
	undo()
	assert.False(t, features.Has("unknown_feature"))
	assert.False(t, features.Has("other"))
}
