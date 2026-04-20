// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIfTypeName(t *testing.T) {
	assert.Equal(t, "ethernet", ifTypeName(6))
	assert.Equal(t, "ppp", ifTypeName(23))
	assert.Equal(t, "loopback", ifTypeName(24))
	assert.Equal(t, "prop_virtual", ifTypeName(53))
	assert.Equal(t, "wifi", ifTypeName(71))
	assert.Equal(t, "tunnel", ifTypeName(131))
	assert.Equal(t, "other_99", ifTypeName(99))
}

func TestClassify_UsesDescrWhenPresent(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			45: {ifType: ifTypeWifi, name: `\DEVICE\{GUID}`, descr: "Intel Wi-Fi 6 AX201"},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(45)
	assert.Equal(t, "Intel Wi-Fi 6 AX201", result.InterfaceName)
	assert.Equal(t, "wifi", result.InterfaceType)
}

func TestClassify_FallsBackToNameWhenDescrEmpty(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			10: {ifType: ifTypeEthernetCSMACD, name: "Ethernet 2", descr: ""},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(10)
	assert.Equal(t, "Ethernet 2", result.InterfaceName)
	assert.Equal(t, "ethernet", result.InterfaceType)
}

func TestClassify_UnknownIndex(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{},
		done:    make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(999)
	assert.Equal(t, "", result.InterfaceName)
	assert.Equal(t, "", result.InterfaceType)
}
