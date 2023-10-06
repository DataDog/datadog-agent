// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLocalDefinedNTPServers(t *testing.T) {
	servers, err := getLocalDefinedNTPServers()
	assert.NoError(t, err)
	assert.NotEmpty(t, servers)
}

func TestGetNptServersFromRegKeyValue(t *testing.T) {
	// time.windows.com,0x9
	// pool.ntp.org time.windows.com time.apple.com time.google.com

	servers, err := getNptServersFromRegKeyValue("time.windows.com,0x9")
	assert.NoError(t, err)
	assert.Equal(t, []string{"time.windows.com"}, servers)

	servers, err = getNptServersFromRegKeyValue("pool.ntp.org time.windows.com")
	assert.NoError(t, err)
	assert.Equal(t, []string{"pool.ntp.org", "time.windows.com"}, servers)

	servers, err = getNptServersFromRegKeyValue("")
	assert.Error(t, err)
	assert.Equal(t, []string(nil), servers)
}
