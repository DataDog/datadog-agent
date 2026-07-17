// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTag_IsTrimmed(t *testing.T) {
	assert.NotContains(t, Tag, "\n")
	assert.NotContains(t, Tag, " ")
	assert.NotEmpty(t, Tag)
}
