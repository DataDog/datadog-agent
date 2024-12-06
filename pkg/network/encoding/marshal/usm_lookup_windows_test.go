// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestUSMLookup(t *testing.T) {
	key := types.NewConnectionKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
	)

	val := new(USMConnectionData[struct{}, any])
	data := make(map[types.ConnectionKey]*USMConnectionData[struct{}, any])
	data[key] = val

	// In windows the USMLookup operation is done only once and using the
	// original tuple order, so in the case below only c1 should match the data
	c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("2.2.2.2"),
		SPort:  60000,
		DPort:  80,
	}}

	c2 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: util.AddressFromString("2.2.2.2"),
		Dest:   util.AddressFromString("1.1.1.1"),
		SPort:  80,
		DPort:  60000,
	}}

	assert.Equal(t, val, USMLookup(c1, data))
	assert.Equal(t, (*USMConnectionData[struct{}, any])(nil), USMLookup(c2, data))
}
