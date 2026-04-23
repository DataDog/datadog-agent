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

func TestClassify_UsesDescrWhenPresent(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			45: {ifType: 71, name: `\DEVICE\{GUID}`, descr: "Intel Wi-Fi 6 AX201"},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(45)
	assert.Equal(t, "Intel Wi-Fi 6 AX201", result.InterfaceName)
	assert.Equal(t, uint32(71), result.InterfaceType)
}

func TestClassify_FallsBackToNameWhenDescrEmpty(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			10: {ifType: 6, name: "Ethernet 2", descr: ""},
		},
		done: make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(10)
	assert.Equal(t, "Ethernet 2", result.InterfaceName)
	assert.Equal(t, uint32(6), result.InterfaceType)
}

func TestClassify_UnknownIndex(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{},
		done:    make(chan struct{}),
	}
	defer c.Close()

	result := c.Classify(999)
	assert.Equal(t, "", result.InterfaceName)
	assert.Equal(t, uint32(0), result.InterfaceType)
}
