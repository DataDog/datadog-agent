// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package path

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimMountRoot(t *testing.T) {
	assert.Equal(t, "/foo/bar", trimMountRoot("/foo/bar", "/"))
	assert.Equal(t, "/foo/bar", trimMountRoot("/foo/bar", ""))
	assert.Equal(t, "/bar", trimMountRoot("/foo/bar", "/foo"))
	assert.Equal(t, "/", trimMountRoot("/foo", "/foo"))
	assert.Equal(t, "/foobar", trimMountRoot("/foobar", "/foo"))
}
