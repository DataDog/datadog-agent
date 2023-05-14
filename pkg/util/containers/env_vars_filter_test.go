// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvFilter_IsIncluded(t *testing.T) {
	filter := newEnvFilter([]string{"FOO", "bar"})

	assert.True(t, filter.IsIncluded("DD_VERSION"))
	assert.True(t, filter.IsIncluded("FOO"))
	assert.True(t, filter.IsIncluded("BAR"))
	assert.False(t, filter.IsIncluded("BAZ"))
}
