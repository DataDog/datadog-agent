// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package probe holds probe related files
package probe

import (
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

func newTestEBPFProbe() *EBPFProbe {
	fieldHandlers := &EBPFFieldHandlers{}
	p := &EBPFProbe{
		fieldHandlers: fieldHandlers,
	}
	p.eventPool = ddsync.NewTypedPool(func() *model.Event {
		return &model.Event{}
	})
	return p
}

func TestDNSAnswerIPNet(t *testing.T) {
	t.Run("accept IPv4-mapped IPv6 AAAA answer", func(t *testing.T) {
		ipNet, ok := dnsAnswerIPNet(layers.DNSResourceRecord{
			Type: layers.DNSTypeAAAA,
			IP:   net.ParseIP("::ffff:192.0.2.1"),
		})

		assert.True(t, ok)
		assert.Equal(t, net.ParseIP("::ffff:192.0.2.1").To16(), ipNet.IP)
		assert.Equal(t, net.CIDRMask(128, 128), ipNet.Mask)
	})
}

func TestGetPoolEvent(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("get event from pool assigns field handlers", func(t *testing.T) {
		event := p.getPoolEvent()

		assert.NotNil(t, event)
		assert.Equal(t, p.fieldHandlers, event.FieldHandlers)
	})

	t.Run("get multiple events from pool", func(t *testing.T) {
		event1 := p.getPoolEvent()
		event2 := p.getPoolEvent()

		assert.NotNil(t, event1)
		assert.NotNil(t, event2)
		assert.Equal(t, p.fieldHandlers, event1.FieldHandlers)
		assert.Equal(t, p.fieldHandlers, event2.FieldHandlers)
	})
}

func TestPutBackPoolEvent(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("put back event with nil ProcessCacheEntry", func(t *testing.T) {
		event := p.getPoolEvent()
		event.ProcessCacheEntry = nil
		// Set some fields to verify they get reset
		event.Type = uint32(model.ExecEventType)
		event.TimestampRaw = 12345

		p.putBackPoolEvent(event)

		// Get a new event from the pool (should be the one we just put back)
		newEvent := p.getPoolEvent()

		// Verify that the event was reset (Type should be 0)
		assert.Equal(t, uint32(0), newEvent.Type)
		assert.Equal(t, uint64(0), newEvent.TimestampRaw)
	})

	t.Run("put back event preserves Os field after reset", func(t *testing.T) {
		event := p.getPoolEvent()
		event.ProcessCacheEntry = nil
		event.Type = uint32(model.ExecEventType)
		event.Os = "linux"

		p.putBackPoolEvent(event)

		// Get a new event from the pool (should be the one we just put back)
		newEvent := p.getPoolEvent()

		// Verify that the Os field is preserved after reset (should be "linux")
		assert.Equal(t, "linux", newEvent.Os)
	})
}

func TestGetAndPutBackPoolEventRoundTrip(t *testing.T) {
	p := newTestEBPFProbe()

	t.Run("round trip get and put back event", func(t *testing.T) {
		// Get an event
		event := p.getPoolEvent()
		assert.NotNil(t, event)
		assert.Equal(t, p.fieldHandlers, event.FieldHandlers)

		// Modify the event
		event.Type = uint32(model.ExecEventType)
		event.TimestampRaw = 99999

		// Put it back
		p.putBackPoolEvent(event)

		// Get another event (should be the same one, reset)
		event2 := p.getPoolEvent()
		assert.Equal(t, uint32(0), event2.Type)
		assert.Equal(t, uint64(0), event2.TimestampRaw)
		assert.Equal(t, p.fieldHandlers, event2.FieldHandlers)
	})
}
