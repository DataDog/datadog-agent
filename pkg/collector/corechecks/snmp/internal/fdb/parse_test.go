// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseQBridgeIndex(t *testing.T) {
	mac, ok := parseQBridgeIndex("1.10.20.30.40.50.60")
	assert.True(t, ok)
	assert.Equal(t, "0a:14:1e:28:32:3c", mac)

	_, ok = parseQBridgeIndex("10.20.30.40.50")
	assert.False(t, ok)

	_, ok = parseQBridgeIndex("1.10.20.30.40.50.256")
	assert.False(t, ok)

	mac, ok = parseQBridgeIndex("4001.0.9.15.9.10.9")
	assert.True(t, ok)
	assert.Equal(t, "00:09:0f:09:0a:09", mac)

	mac, ok = parseQBridgeIndex("196608.0.16.219.255.16.1")
	assert.True(t, ok)
	assert.Equal(t, "00:10:db:ff:10:01", mac)

	mac, ok = parseQBridgeIndex("1.6.0.12.41.21.230.31")
	assert.True(t, ok)
	assert.Equal(t, "00:0c:29:15:e6:1f", mac)
}

func TestParseBridgeIndex(t *testing.T) {
	mac, ok := parseBridgeIndex("10.20.30.40.50.60")
	assert.True(t, ok)
	assert.Equal(t, "0a:14:1e:28:32:3c", mac)

	_, ok = parseBridgeIndex("1.10.20.30.40.50.60")
	assert.False(t, ok)

	_, ok = parseBridgeIndex("1")
	assert.False(t, ok)
}

func TestMACFilters(t *testing.T) {
	assert.True(t, isZeroMAC("00:00:00:00:00:00"))
	assert.True(t, isBroadcastMAC("ff:ff:ff:ff:ff:ff"))
	assert.True(t, isMulticastMAC("01:00:5e:00:00:01"))
	assert.False(t, isMulticastMAC("0a:14:1e:28:32:3c"))
}
