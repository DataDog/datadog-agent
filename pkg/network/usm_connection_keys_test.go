// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestWithKey(t *testing.T) {
	t.Run("without NAT", func(t *testing.T) {
		c := ConnectionStats{ConnectionTuple: ConnectionTuple{
			Source: util.AddressFromString("10.1.1.1"),
			Dest:   util.AddressFromString("10.2.2.2"),
			SPort:  60000,
			DPort:  80,
		}}

		shouldGenerateKeys(t, c,
			types.NewConnectionKey(c.Source, c.Dest, c.SPort, c.DPort),
			types.NewConnectionKey(c.Dest, c.Source, c.DPort, c.SPort),
		)
	})

	t.Run("with NAT", func(t *testing.T) {
		c := ConnectionStats{ConnectionTuple: ConnectionTuple{
			Source: util.AddressFromString("10.1.1.1"),
			Dest:   util.AddressFromString("10.2.2.2"),
			SPort:  60000,
			DPort:  80,
		},
			IPTranslation: &IPTranslation{
				ReplSrcIP:   util.AddressFromString("3.3.3.3"),
				ReplDstIP:   util.AddressFromString("4.4.4.4"),
				ReplSrcPort: 50000,
				ReplDstPort: 8080,
			},
		}

		shouldGenerateKeys(t, c,
			types.NewConnectionKey(c.Source, c.Dest, c.SPort, c.DPort),
			types.NewConnectionKey(c.Dest, c.Source, c.DPort, c.SPort),
			types.NewConnectionKey(c.IPTranslation.ReplSrcIP, c.IPTranslation.ReplDstIP, c.IPTranslation.ReplSrcPort, c.IPTranslation.ReplDstPort),
			types.NewConnectionKey(c.IPTranslation.ReplDstIP, c.IPTranslation.ReplSrcIP, c.IPTranslation.ReplDstPort, c.IPTranslation.ReplSrcPort),
		)
	})
}

func shouldGenerateKeys(t *testing.T, c ConnectionStats, expectedKeys ...types.ConnectionKey) {
	var generatedKeys []types.ConnectionKey

	WithKey(c, func(key types.ConnectionKey) bool {
		generatedKeys = append(generatedKeys, key)
		return false
	})

	assert.ElementsMatch(t, expectedKeys, generatedKeys)
}
