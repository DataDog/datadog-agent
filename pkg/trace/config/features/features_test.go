// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package features

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeatures(t *testing.T) {
	Set("a, b ,c")
	assert.True(t, Has("a"))
	assert.True(t, Has("b"))
	assert.True(t, Has("c"))
	assert.ElementsMatch(t, All(), []string{"a", "b", "c"})
}
