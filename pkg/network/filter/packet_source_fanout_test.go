// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package filter

import (
	"testing"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestPacketSourceFanoutDistributesAndUnsubscribes(t *testing.T) {
	source := &fanoutTestPacketSource{}
	fanout := NewPacketSourceFanout(source)
	var first, second, primary int
	fanout.Subscribe(func([]byte, PacketInfo, time.Time) error {
		first++
		return nil
	})
	unsubscribe := fanout.Subscribe(func([]byte, PacketInfo, time.Time) error {
		second++
		return nil
	})

	require.NoError(t, fanout.VisitPackets(func([]byte, PacketInfo, time.Time) error {
		primary++
		return nil
	}))
	unsubscribe()
	require.NoError(t, fanout.VisitPackets(func([]byte, PacketInfo, time.Time) error {
		primary++
		return nil
	}))
	fanout.Close()
	fanout.Close()

	require.Equal(t, 2, first)
	require.Equal(t, 1, second)
	require.Equal(t, 2, primary)
	require.Equal(t, 1, source.closes)
}

type fanoutTestPacketSource struct {
	closes int
}

func (s *fanoutTestPacketSource) VisitPackets(visitor func([]byte, PacketInfo, time.Time) error) error {
	return visitor([]byte{1, 2, 3}, fanoutTestPacketInfo{}, time.Now())
}

func (s *fanoutTestPacketSource) LayerType() gopacket.LayerType {
	return layers.LayerTypeEthernet
}

func (s *fanoutTestPacketSource) Close() {
	s.closes++
}

type fanoutTestPacketInfo struct{}

func (fanoutTestPacketInfo) PacketType() uint8                 { return 0 }
func (fanoutTestPacketInfo) LinkLayerType() gopacket.LayerType { return layers.LayerTypeEthernet }
func (fanoutTestPacketInfo) OriginalLength() int               { return 3 }
func (fanoutTestPacketInfo) CapturedLength() int               { return 3 }
