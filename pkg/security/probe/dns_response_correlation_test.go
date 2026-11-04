// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"net"
	"slices"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dns"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	securityprofile "github.com/DataDog/datadog-agent/pkg/security/security_profile"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

const (
	testQTypeA    = uint16(1)
	testQTypeAAAA = uint16(28)
)

func TestNewCorrelatedDNSEvent(t *testing.T) {
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	ev := model.NewFakeEvent()
	ts := time.Unix(1757520000, 0)

	question := model.DNSQuestion{Name: "one.one.one.one", Type: testQTypeA, Class: 1}
	response := &model.DNSResponse{
		ResponseCode: 0,
		IPs:          []net.IPNet{{IP: net.IPv4(1, 1, 1, 1), Mask: net.CIDRMask(32, 32)}},
		CNames:       []string{"cdn.example.com"},
	}

	newCorrelatedDNSEvent(ev, entry, 0x1234, question, response, ts, 99)

	assert.Equal(t, model.DNSEventType, ev.GetEventType())
	// derived in user space, not received from the kernel
	assert.Equal(t, model.EventSourceRelated, ev.Source)
	assert.Equal(t, ts, ev.Timestamp)
	assert.Equal(t, uint64(99), ev.TimestampRaw)
	assert.Same(t, entry, ev.ProcessCacheEntry)
	assert.Same(t, &entry.ProcessContext, ev.ProcessContext)
	assert.Equal(t, uint16(0x1234), ev.DNS.ID)
	assert.Equal(t, question, ev.DNS.Question)
	assert.Same(t, response, ev.DNS.Response)
}

// capturedDNSEvent holds everything the assertions need from a correlated event. It has to be
// copied out inside the stub: correlateDNSResponseForActivityDump returns the event to the pool as
// soon as it returns, and the pool hands the same *model.Event out again.
type capturedDNSEvent struct {
	eventType    model.EventType
	source       string
	entry        *model.ProcessCacheEntry
	id           uint16
	question     model.DNSQuestion
	responseCode uint8
	ips          []net.IPNet
	cnames       []string
}

// stubProfileManager records the events the correlation path hands to the profile manager.
//
// securityprofile.ProfileManager is a wide interface, so it is embedded rather than implemented:
// only ProcessEvent is defined, and any other method the correlation path might start calling
// would panic on the nil embedded interface rather than pass silently.
type stubProfileManager struct {
	securityprofile.ProfileManager

	events []capturedDNSEvent
}

func (m *stubProfileManager) ProcessEvent(ev *model.Event) {
	captured := capturedDNSEvent{
		eventType: ev.GetEventType(),
		source:    ev.Source,
		entry:     ev.ProcessCacheEntry,
		id:        ev.DNS.ID,
		question:  ev.DNS.Question,
	}
	if ev.DNS.Response != nil {
		captured.responseCode = ev.DNS.Response.ResponseCode
		captured.ips = slices.Clone(ev.DNS.Response.IPs)
		captured.cnames = slices.Clone(ev.DNS.Response.CNames)
	}
	m.events = append(m.events, captured)
}

// newTestCorrelationProbe builds the smallest EBPFProbe correlateDNSResponseForActivityDump needs:
// an event pool, field handlers, a time resolver, the tracker and a profile manager. NewEBPFProbe
// is not usable here, it attaches to the kernel.
func newTestCorrelationProbe(t *testing.T) (*EBPFProbe, *stubProfileManager) {
	t.Helper()

	timeResolver, err := ktime.NewResolver()
	require.NoError(t, err)

	fieldHandlers, err := NewEBPFFieldHandlers(&config.Config{
		Probe:           &pconfig.Config{},
		RuntimeSecurity: &config.RuntimeSecurityConfig{},
	}, nil, "test-host", nil)
	require.NoError(t, err)

	profileManager := &stubProfileManager{}

	dnsRequests, err := dns.NewRequestTracker()
	require.NoError(t, err)

	p := &EBPFProbe{
		Resolvers:      &resolvers.EBPFResolvers{TimeResolver: timeResolver},
		fieldHandlers:  fieldHandlers,
		profileManager: profileManager,
		dnsRequests:    dnsRequests,
	}
	p.eventPool = ddsync.NewTypedPool(func() *model.Event {
		return newEBPFEvent(p.fieldHandlers)
	})

	return p, profileManager
}

func newTestDNSResponseLayer(id uint16, name string, qtype layers.DNSType) *layers.DNS {
	return &layers.DNS{
		ID:           id,
		QR:           true,
		ResponseCode: layers.DNSResponseCodeNoErr,
		Questions: []layers.DNSQuestion{{
			Name:  []byte(name),
			Type:  qtype,
			Class: layers.DNSClassIN,
		}},
	}
}

func TestCorrelateDNSResponseForActivityDumpAttributesToTheRequester(t *testing.T) {
	p, profileManager := newTestCorrelationProbe(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)

	p.dnsRequests.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, time.Now())

	ips := []net.IPNet{{IP: net.IPv4(1, 1, 1, 1), Mask: net.CIDRMask(32, 32)}}
	cnames := []string{"cdn.example.com"}
	p.correlateDNSResponseForActivityDump(newTestDNSResponseLayer(0x1234, "one.one.one.one", layers.DNSTypeA), ips, cnames)

	require.Len(t, profileManager.events, 1)
	got := profileManager.events[0]

	assert.Equal(t, model.DNSEventType, got.eventType)
	// derived in user space, not received from the kernel
	assert.Equal(t, model.EventSourceRelated, got.source)
	assert.Same(t, entry, got.entry, "the response must be attributed to the process that asked")
	assert.Equal(t, uint16(0x1234), got.id)
	assert.Equal(t, model.DNSQuestion{Name: "one.one.one.one", Type: testQTypeA, Class: 1}, got.question)
	assert.Equal(t, uint8(0), got.responseCode)
	assert.Equal(t, ips, got.ips)
	assert.Equal(t, cnames, got.cnames)
}

// The tree indexes DNS nodes by the exact question name, so a response echoing the question in
// another case must be filed under the request's spelling, or it would create a second node.
func TestCorrelateDNSResponseForActivityDumpKeepsTheRequestSpelling(t *testing.T) {
	p, profileManager := newTestCorrelationProbe(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)

	p.dnsRequests.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, time.Now())

	p.correlateDNSResponseForActivityDump(newTestDNSResponseLayer(0x1234, "OnE.oNe.ONE.one", layers.DNSTypeA),
		[]net.IPNet{{IP: net.IPv4(1, 1, 1, 1), Mask: net.CIDRMask(32, 32)}}, nil)

	require.Len(t, profileManager.events, 1)
	assert.Equal(t, "one.one.one.one", profileManager.events[0].question.Name)
}

// A response the tracker knows nothing about is dropped.
func TestCorrelateDNSResponseForActivityDumpIgnoresUnmatchedResponse(t *testing.T) {
	t.Run("nothing tracked", func(t *testing.T) {
		p, profileManager := newTestCorrelationProbe(t)

		p.correlateDNSResponseForActivityDump(
			newTestDNSResponseLayer(0x1234, "one.one.one.one", layers.DNSTypeA),
			[]net.IPNet{{IP: net.IPv4(1, 1, 1, 1), Mask: net.CIDRMask(32, 32)}}, nil)

		assert.Empty(t, profileManager.events)
	})

	t.Run("another question tracked", func(t *testing.T) {
		p, profileManager := newTestCorrelationProbe(t)
		entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)

		p.dnsRequests.RecordRequest(0x1234, "one.one.one.one", testQTypeA, entry, time.Now())

		p.correlateDNSResponseForActivityDump(
			newTestDNSResponseLayer(0x4321, "example.com", layers.DNSTypeA),
			[]net.IPNet{{IP: net.IPv4(93, 184, 216, 34), Mask: net.CIDRMask(32, 32)}}, nil)

		assert.Empty(t, profileManager.events)
	})
}

// NODATA: a NOERROR response with no answers has nothing to enrich a dump with, so it must not
// reach the profile manager even when its question is being tracked.
func TestCorrelateDNSResponseForActivityDumpSkipsEmptyAnswers(t *testing.T) {
	p, profileManager := newTestCorrelationProbe(t)
	entry := model.NewPlaceholderProcessCacheEntry(42, 42, false)

	p.dnsRequests.RecordRequest(0x1234, "one.one.one.one", testQTypeAAAA, entry, time.Now())

	p.correlateDNSResponseForActivityDump(newTestDNSResponseLayer(0x1234, "one.one.one.one", layers.DNSTypeAAAA), nil, nil)

	assert.Empty(t, profileManager.events)
}

// The correlated event goes straight back to the pool, so the next one must not inherit anything
// from it. Reusing the pooled event is the whole reason the insert path deep-copies.
func TestCorrelateDNSResponseForActivityDumpReusesPooledEvents(t *testing.T) {
	p, profileManager := newTestCorrelationProbe(t)
	a := model.NewPlaceholderProcessCacheEntry(42, 42, false)
	b := model.NewPlaceholderProcessCacheEntry(43, 43, false)
	now := time.Now()

	p.dnsRequests.RecordRequest(0x1234, "one.one.one.one", testQTypeA, a, now)
	p.dnsRequests.RecordRequest(0x4321, "example.com", testQTypeA, b, now)

	first := []net.IPNet{{IP: net.IPv4(1, 1, 1, 1), Mask: net.CIDRMask(32, 32)}}
	p.correlateDNSResponseForActivityDump(newTestDNSResponseLayer(0x1234, "one.one.one.one", layers.DNSTypeA), first, []string{"cdn.example.com"})

	second := []net.IPNet{{IP: net.IPv4(93, 184, 216, 34), Mask: net.CIDRMask(32, 32)}}
	p.correlateDNSResponseForActivityDump(newTestDNSResponseLayer(0x4321, "example.com", layers.DNSTypeA), second, nil)

	require.Len(t, profileManager.events, 2)
	assert.Same(t, a, profileManager.events[0].entry)
	assert.Same(t, b, profileManager.events[1].entry)
	assert.Equal(t, "example.com", profileManager.events[1].question.Name)
	assert.Equal(t, second, profileManager.events[1].ips)
	assert.Empty(t, profileManager.events[1].cnames, "the second event must not inherit the first one's CNAMEs")
}
