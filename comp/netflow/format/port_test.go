// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPort(t *testing.T) {
	assert.Equal(t, "65535", Port(math.MaxUint16))
	assert.Equal(t, "10", Port(10))
	assert.Equal(t, "0", Port(0))
	assert.Equal(t, "*", Port(-1))
	assert.Equal(t, "invalid", Port(-10))
}
