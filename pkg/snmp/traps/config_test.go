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
		Community: []string{"public"},
	}
	params, err := config.BuildParams()
	assert.NoError(t, err)

	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
}

func TestConfigPort(t *testing.T) {
	t.Run("err-required", func(t *testing.T) {
		config := TrapListenerConfig{}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: []string{"public"},
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)
		assert.Equal(t, 162, int(params.Port))
	})
}

func TestConfigV2(t *testing.T) {
	t.Run("community", func(t *testing.T) {
		config := TrapListenerConfig{
			Port:      162,
			Community: []string{"public"},
		}
		params, err := config.BuildParams()
		assert.NoError(t, err)

		assert.Equal(t, gosnmp.Version2c, params.Version)
		assert.Equal(t, "", params.Community) // Not copied over, we validate community strings manually
	})

	t.Run("err-community-missing", func(t *testing.T) {
		config := TrapListenerConfig{
			Port: 162,
		}
		_, err := config.BuildParams()
		assert.Error(t, err)
	})
}
