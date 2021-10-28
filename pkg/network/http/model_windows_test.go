// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm
// +build windows,npm

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLatency(t *testing.T) {
	tx := httpTX{
		ResponseLastSeen: 2e6,
		RequestStarted:   1e6,
	}
	// quantization brings it down
	assert.Equal(t, 999424.0, tx.RequestLatency())
}
