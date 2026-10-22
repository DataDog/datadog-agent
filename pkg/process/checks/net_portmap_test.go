// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || windows

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/remoteservice"
)

func TestGetListeningPortToPIDMapReturnsCopy(t *testing.T) {
	getListeningPortToPIDMap()
	if cachedPortMap == nil {
		t.Skip("listening ports could not be polled")
	}

	key := remoteservice.ListenKey{IP: "203.0.113.1", Port: 65535}
	for range 3 {
		m := getListeningPortToPIDMap()
		require.NotNil(t, m)
		m[key] = 1
		assert.NotContains(t, cachedPortMap, key, "mutating the returned map must not modify the cache")
	}
}
