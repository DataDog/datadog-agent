// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"testing"
	"time"

	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	configure(t, Config{
		Port:             1234,
		CommunityStrings: []string{"public"},
	})
	c, err := ReadConfig()
	assert.NoError(t, err)
	assert.Equal(t, uint16(1234), c.Port)
	assert.Equal(t, 5.0*time.Second, c.StopTimeout)

	params := c.BuildV2Params()
	assert.Equal(t, uint16(1234), params.Port)
	assert.Equal(t, gosnmp.Version2c, params.Version)
	assert.Equal(t, "udp", params.Transport)
	assert.NotNil(t, params.Logger)
}

func TestDefaultPort(t *testing.T) {
	configure(t, Config{
		CommunityStrings: []string{"public"},
	})
	c, err := ReadConfig()
	assert.NoError(t, err)
	assert.Equal(t, uint16(162), c.Port)
}

func TestCommunityStringsEmpty(t *testing.T) {
	configure(t, Config{
		CommunityStrings: []string{},
	})
	_, err := ReadConfig()
	assert.Error(t, err)
}

func TestCommunityStringsMissing(t *testing.T) {
	configure(t, Config{})
	_, err := ReadConfig()
	assert.Error(t, err)
}
