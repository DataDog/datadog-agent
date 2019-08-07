// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

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

	assert.Equal(t, []string{"time.windows.com"}, getNptServersFromRegKeyValue("time.windows.com,0x9"))
	assert.Equal(t, []string{"pool.ntp.org", "time.windows.com"}, getNptServersFromRegKeyValue("pool.ntp.org time.windows.com"))
	assert.Equal(t, []string(nil), getNptServersFromRegKeyValue(""))
}
