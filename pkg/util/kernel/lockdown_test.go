// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLockdown(t *testing.T) {
	mode := getLockdownMode(`none integrity [confidentiality]`)
	assert.Equal(t, Confidentiality, mode)

	mode = getLockdownMode(`none [integrity] confidentiality`)
	assert.Equal(t, Integrity, mode)

	mode = getLockdownMode(`[none] integrity confidentiality`)
	assert.Equal(t, None, mode)

	mode = getLockdownMode(`none integrity confidentiality`)
	assert.Equal(t, Unknown, mode)

	mode = getLockdownMode(`none integrity confidentiality [aaa]`)
	assert.Equal(t, Unknown, mode)
}
