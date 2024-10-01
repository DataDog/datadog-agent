// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Contains BSD-2-Clause code (c) 2015-present Andrea Barberio

package net

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestICMPMarshalBinary(t *testing.T) {
	want := []byte{
		11, // ICMP time exceeded
		0,
		0xf4, 0xff, // checksum
		0, 0, 0, 0, // unused
	}
	icmp := ICMP{
		Type: ICMPTimeExceeded,
		Code: 0,
	}
	b, err := icmp.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, want, b)
}

func TestICMPUnmarshalBinaryBinary(t *testing.T) {
	b := []byte{
		11, // ICMP time exceeded
		0,
		0xf4, 0xff, // checksum
		0, 0, 0, 0, // unused
		// payload
		0xde, 0xad, 0xc0, 0xde,
	}
	var i ICMP
	err := i.UnmarshalBinary(b)
	require.NoError(t, err)
	assert.Equal(t, ICMPTimeExceeded, i.Type)
	assert.Equal(t, ICMPCode(0), i.Code)
}
