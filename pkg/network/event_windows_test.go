// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var englishOut = `
Protocol tcp Dynamic Port Range
---------------------------------
Start Port      : 49152
Number of Ports : 16384`

//nolint:misspell // misspell only handles english
var frenchOut = `
Plage de ports dynamique du protocole tcp
---------------------------------
Port de démarrage   : 49152
Nombre de ports     : 16384
`

func TestNetshParse(t *testing.T) {
	t.Run("english", func(t *testing.T) {
		low, hi, err := parseNetshOutput(englishOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
	t.Run("french", func(t *testing.T) {
		low, hi, err := parseNetshOutput(frenchOut)
		require.NoError(t, err)
		assert.Equal(t, uint16(49152), low)
		assert.Equal(t, uint16(65535), hi)
	})
}

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

	assert.Len(t, keyTuples, 1, "Expected different number of key tuples")
	assert.True(t, slices.ContainsFunc(keyTuples, func(keyTuple types.ConnectionKey) bool {
		sourceAddressLow, sourceAddressHigh := util.ToLowHigh(sourceAddress)
		destinationAddressLow, destinationAddressHigh := util.ToLowHigh(destinationAddress)
		return (keyTuple.SrcIPLow == sourceAddressLow) && (keyTuple.SrcIPHigh == sourceAddressHigh) &&
			(keyTuple.DstIPLow == destinationAddressLow) && (keyTuple.DstIPHigh == destinationAddressHigh) &&
			(keyTuple.SrcPort == sourcePort) && (keyTuple.DstPort == destinationPort)
	}), "Missing original connection")

}
