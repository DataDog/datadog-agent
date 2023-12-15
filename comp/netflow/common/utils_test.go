// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxUint64(t *testing.T) {
	assert.Equal(t, uint64(10), Max(uint64(10), uint64(5)))
	assert.Equal(t, uint64(10), Max(uint64(5), uint64(10)))
}

func TestMinUint64(t *testing.T) {
	assert.Equal(t, uint64(5), Min(uint64(10), uint64(5)))
	assert.Equal(t, uint64(5), Min(uint64(5), uint64(10)))
}

func TestMaxUint16(t *testing.T) {
	assert.Equal(t, uint16(10), Max(uint16(10), uint16(5)))
	assert.Equal(t, uint16(10), Max(uint16(5), uint16(10)))
}

func TestMaxUint32(t *testing.T) {
	assert.Equal(t, uint32(10), Max(uint32(10), uint32(5)))
	assert.Equal(t, uint32(10), Max(uint32(5), uint32(10)))
}
