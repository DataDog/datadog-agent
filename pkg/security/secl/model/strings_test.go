// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsPrintable(t *testing.T) {
	assert.Equal(t, true, IsPrintable("A-B"))
	assert.Equal(t, true, IsPrintable("A/B"))
	assert.Equal(t, false, IsPrintable("\n"))
	assert.Equal(t, false, IsPrintable("\u001d"))
}

func TestIsPrintableASCII(t *testing.T) {
	assert.Equal(t, true, IsPrintableASCII("A-B"))
	assert.Equal(t, true, IsPrintableASCII("A/B"))
	assert.Equal(t, true, IsPrintableASCII("/dev/pts2"))
	assert.Equal(t, false, IsPrintableASCII("\n"))
	assert.Equal(t, false, IsPrintableASCII("\u001d"))
}
