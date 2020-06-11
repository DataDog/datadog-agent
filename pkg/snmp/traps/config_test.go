// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"testing"

	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestConfigCommon(t *testing.T) {
	config := TrapListenerConfig{
		Port:      162,
		Community: "public",
	}
	params, err := config.BuildParams()
	assert.NoError(t, err)

	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
}

func TestConfigPort(t *testing.T) {
	t.Run("err-required", func(t *testing.T) {
		config := TrapListenerConfig{
			Community: "public",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)
		assert.Equal(t, 162, int(params.Port))
	})
}

func TestConfigVersion(t *testing.T) {
	t.Run("default-is-v2", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("explicit-v1", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Version:   "1",
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version1, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("err-invalid-version", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Version:   "42",
			Community: "public",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("err-v3-not-supported", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Version:   "3",
			Community: "public",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})
}

func TestConfigV2(t *testing.T) {
	t.Run("community", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: "public",
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "public", params.Community)
		assert.Equal(t, 0, int(params.SecurityModel))
		assert.Nil(t, params.SecurityParameters)
	})

	t.Run("err-community-missing", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:    162,
			Version: "2c",
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})
}
