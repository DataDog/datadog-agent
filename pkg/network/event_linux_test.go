// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
)

/*
 * these tests split out from Windows due to the change that Windows
 * no longer normalizes connections, so there should always be a 1:1 match
 *
 * be aware of changes here that should be reflected into event_test_windows.go
 */
func TestKeyTuplesFromConn(t *testing.T) {
	sourceAddress := util.AddressFromString("1.2.3.4")
	sourcePort := uint16(1234)
	destinationAddress := util.AddressFromString("5.6.7.8")
	destinationPort := uint16(5678)

	connectionStats := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: sourceAddress,
		SPort:  sourcePort,
		Dest:   destinationAddress,
		DPort:  destinationPort,
	}}
	keyTuples := ConnectionKeysFromConnectionStats(connectionStats)

	assert.Len(t, keyTuples, 2, "Expected different number of key tuples")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == destinationPort) && (keyTuple.DstPort == sourcePort)
	}), "Missing flipped connection")
}

func TestKeyTuplesFromConnNAT(t *testing.T) {
	sourceAddress := util.AddressFromString("1.2.3.4")
	sourcePort := uint16(1234)
	destinationAddress := util.AddressFromString("5.6.7.8")
	destinationPort := uint16(5678)

	natSourceAddress := util.AddressFromString("10.20.30.40")
	natSourcePort := uint16(4321)
	natDestinationAddress := util.AddressFromString("50.60.70.80")
	natDestinationPort := uint16(8765)

	connectionStats := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: sourceAddress,
		Dest:   destinationAddress,
		SPort:  sourcePort,
		DPort:  destinationPort,
	},
		IPTranslation: &IPTranslation{
			ReplSrcIP:   natSourceAddress,
			ReplDstIP:   natDestinationAddress,
			ReplSrcPort: natSourcePort,
			ReplDstPort: natDestinationPort,
		},
	}
	keyTuples := ConnectionKeysFromConnectionStats(connectionStats)

	// Expecting 2 non NAT'd keys and 2 NAT'd keys
	assert.Len(t, keyTuples, 4, "Expected different number of key tuples")

	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == destinationPort) && (keyTuple.DstPort == sourcePort)
	}), "Missing flipped connection")

	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(natSourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(natDestinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == natSourcePort) && (keyTuple.DstPort == natDestinationPort)
	}), "Missing NAT'd connection")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(natSourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(natDestinationAddress)
		return (keyTuple.SrcIPLow == destinationAddressLow) && (keyTuple.SrcIPHigh == destinationAddressHigh) &&
			(keyTuple.DstIPLow == sourceAddressLow) && (keyTuple.DstIPHigh == sourceAddressHigh) &&
			(keyTuple.SrcPort == natDestinationPort) && (keyTuple.DstPort == natSourcePort)
	}), "Missing flipped NAT'd connection")
}
