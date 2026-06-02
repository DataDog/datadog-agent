// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profile manager related files
package securityprofile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewImageExcluder(t *testing.T) {
	t.Run("empty list returns nil excluder", func(t *testing.T) {
		e, err := newImageExcluder(nil)
		require.NoError(t, err)
		assert.Nil(t, e)

		e, err = newImageExcluder([]string{})
		require.NoError(t, err)
		assert.Nil(t, e)
	})

	t.Run("rejects malformed entries", func(t *testing.T) {
		cases := []string{
			"no-colon",
			":missing-name",
			"missing-tag:",
			":",
			"",
		}
		for _, entry := range cases {
			t.Run(entry, func(t *testing.T) {
				_, err := newImageExcluder([]string{entry})
				assert.Error(t, err)
			})
		}
	})

	t.Run("collects multiple tags per image", func(t *testing.T) {
		e, err := newImageExcluder([]string{"ubuntu:20.04", "ubuntu:22.04"})
		require.NoError(t, err)
		require.NotNil(t, e)

		assert.True(t, e.IsExcluded("ubuntu", "20.04"))
		assert.True(t, e.IsExcluded("ubuntu", "22.04"))
		assert.False(t, e.IsExcluded("ubuntu", "18.04"))
	})

	t.Run("registry with port parses on last colon", func(t *testing.T) {
		e, err := newImageExcluder([]string{"registry.io:5000/team/app:v1"})
		require.NoError(t, err)
		require.NotNil(t, e)

		assert.True(t, e.IsExcluded("registry.io:5000/team/app", "v1"))
		assert.False(t, e.IsExcluded("registry.io", "5000/team/app:v1"))
	})
}

func TestImageExcluder_IsExcluded(t *testing.T) {
	e, err := newImageExcluder([]string{
		"ubuntu:*",
		"alpine:3.18",
		"mycorp/internal:dev",
	})
	require.NoError(t, err)
	require.NotNil(t, e)

	tests := []struct {
		name     string
		image    string
		tag      string
		excluded bool
	}{
		{"wildcard matches any tag", "ubuntu", "20.04", true},
		{"wildcard matches empty tag", "ubuntu", "", true},
		{"exact tag match", "alpine", "3.18", true},
		{"exact tag mismatch", "alpine", "3.19", false},
		{"image with slash exact match", "mycorp/internal", "dev", true},
		{"image with slash tag mismatch", "mycorp/internal", "prod", false},
		{"unknown image", "nginx", "latest", false},
		{"empty image never matches", "", "20.04", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.excluded, e.IsExcluded(tc.image, tc.tag))
		})
	}
}

func TestImageExcluder_NilReceiver(t *testing.T) {
	var e *imageExcluder
	assert.False(t, e.IsExcluded("ubuntu", "20.04"))
	assert.False(t, e.IsExcluded("", ""))
}
