// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildDockerFilterOddNumber(t *testing.T) {
	opt, err := buildDockerFilter("test")
	assert.NotNil(t, err)
	assert.Equal(t, 0, opt.Filters.Len())
}

func TestBuildDockerFilterOK(t *testing.T) {
	opt, err := buildDockerFilter("k1", "v1", "k2", "v2")
	assert.Nil(t, err)
	assert.Equal(t, 2, opt.Filters.Len())
	assert.Equal(t, []string{"v1"}, opt.Filters.Get("k1"))
	assert.Equal(t, []string{"v2"}, opt.Filters.Get("k2"))
}

func TestBuildDockerFilterEmptyOK(t *testing.T) {
	opt, err := buildDockerFilter()
	assert.Nil(t, err)
	assert.Equal(t, 0, opt.Filters.Len())
}
