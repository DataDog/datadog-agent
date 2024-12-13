// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestUSMLookup(t *testing.T) {
	t.Run("without NAT", func(t *testing.T) {
		key := types.NewConnectionKey(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			60000,
			80,
		)

		// The value doesn't matter for the purposes of this test
		// We only want to make sure that this object is returned during lookups
		val := new(USMConnectionData[struct{}, any])

		data := make(map[types.ConnectionKey]*USMConnectionData[struct{}, any])
		data[key] = val

		// Assert that c1 and c2 (which are symmetrical) "link" to the same aggregation
		c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("1.1.1.1"),
			Dest:   util.AddressFromString("2.2.2.2"),
			SPort:  60000,
			DPort:  80,
		}}
		assert.Equal(t, val, USMLookup(c1, data))

		c2 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("2.2.2.2"),
			Dest:   util.AddressFromString("1.1.1.1"),
			SPort:  80,
			DPort:  60000,
		}}
		assert.Equal(t, val, USMLookup(c2, data))
	})

	t.Run("with NAT", func(t *testing.T) {
		key := types.NewConnectionKey(
			util.AddressFromString("3.3.3.3"),
			util.AddressFromString("4.4.4.4"),
			50000,
			8080,
		)

		val := new(USMConnectionData[struct{}, any])
		data := make(map[types.ConnectionKey]*USMConnectionData[struct{}, any])
		data[key] = val

		// Assert that c1 and c2 (which are symmetrical) "link" to the same aggregation
		c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("1.1.1.1"),
			Dest:   util.AddressFromString("2.2.2.2"),
			SPort:  60000,
			DPort:  80,
		},
			IPTranslation: &network.IPTranslation{
				ReplSrcIP:   util.AddressFromString("3.3.3.3"),
				ReplDstIP:   util.AddressFromString("4.4.4.4"),
				ReplSrcPort: 50000,
				ReplDstPort: 8080,
			},
		}
		assert.Equal(t, val, USMLookup(c1, data))

		c2 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("2.2.2.2"),
			Dest:   util.AddressFromString("1.1.1.1"),
			SPort:  80,
			DPort:  60000,
		},
			IPTranslation: &network.IPTranslation{
				ReplSrcIP:   util.AddressFromString("4.4.4.4"),
				ReplDstIP:   util.AddressFromString("3.3.3.3"),
				ReplSrcPort: 8080,
				ReplDstPort: 50000,
			},
		}
		assert.Equal(t, val, USMLookup(c2, data))
	})
}
