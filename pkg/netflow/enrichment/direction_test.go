// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemapDirection(t *testing.T) {
	assert.Equal(t, "ingress", RemapDirection(uint32(0)))
	assert.Equal(t, "egress", RemapDirection(uint32(1)))
	assert.Equal(t, "ingress", RemapDirection(uint32(99))) // invalid direction will default to ingress
}
