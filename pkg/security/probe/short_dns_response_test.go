// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

package probe

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/netip"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dns"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// recordingEventHandler records the type of every event the probe dispatches.
type recordingEventHandler struct {
	eventTypes []model.EventType
}

func (h *recordingEventHandler) HandleEvent(event *model.Event) {
	h.eventTypes = append(h.eventTypes, event.GetEventType())
}

// recordingCustomEventHandler keeps the custom events the probe dispatches, to be marshaled later.
type recordingCustomEventHandler struct {
	ruleIDs []string
	events  []*events.CustomEvent
}

func (h *recordingCustomEventHandler) HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent) {
	h.ruleIDs = append(h.ruleIDs, rule.ID)
	h.events = append(h.events, event)
}

// shortDNSResponseRecord builds a kernel record of a short DNS response: the event header
// followed directly by the DNS packet, without any process, span or cgroup context.
func shortDNSResponseRecord(packet []byte) []byte {
	buf := make([]byte, 16+len(packet))
	binary.NativeEndian.PutUint64(buf[0:8], 1000000000)
	binary.NativeEndian.PutUint32(buf[8:12], uint32(model.ShortDNSResponseEventType))
	copy(buf[16:], packet)
	return buf
}

// undecodableDNSResponse returns a DNS response header announcing more questions than the
// packet holds, followed by enough bytes to cover the size of the generic event contexts.
func undecodableDNSResponse() []byte {
	packet := make([]byte, 12, 12+128)
	binary.BigEndian.PutUint16(packet[0:2], 0x0102) // id
	binary.BigEndian.PutUint16(packet[2:4], 0x8180) // response, no error
	binary.BigEndian.PutUint16(packet[4:6], 0xffff) // questions
	return append(packet, bytes.Repeat([]byte{0x01}, 128)...)
}

func validDNSResponse(t *testing.T, name string, ip net.IP) []byte {
	t.Helper()

	response := &layers.DNS{
		ID:           0x0102,
		QR:           true,
		ResponseCode: layers.DNSResponseCodeNoErr,
		Questions:    []layers.DNSQuestion{{Name: []byte(name), Type: layers.DNSTypeA, Class: layers.DNSClassIN}},
		Answers: []layers.DNSResourceRecord{{
			Name:  []byte(name),
			Type:  layers.DNSTypeA,
			Class: layers.DNSClassIN,
			TTL:   60,
			IP:    ip,
		}},
	}

	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, response.SerializeTo(buf, gopacket.SerializeOptions{FixLengths: true}))
	return buf.Bytes()
}

func newShortDNSTestProbe(t *testing.T, dnsResolutionEnabled bool) (*EBPFProbe, *recordingEventHandler, *recordingCustomEventHandler) {
	t.Helper()

	p := newFuzzEBPFProbe(t)
	p.config.Probe.DNSResolutionEnabled = dnsResolutionEnabled
	p.config.Probe.DNSResolverCacheSize = 16
	p.config.RuntimeSecurity.InternalMonitoringEnabled = true

	dnsResolver, err := dns.NewDNSResolver(p.config.Probe, nil)
	require.NoError(t, err)
	p.Resolvers.DNSResolver = dnsResolver

	handler := &recordingEventHandler{}
	require.NoError(t, p.probe.AddEventHandler(handler))
	customHandler := &recordingCustomEventHandler{}
	require.NoError(t, p.probe.AddCustomEventHandler(customHandler))

	return p, handler, customHandler
}

func TestShortDNSResponseUndecodableIsNotDispatched(t *testing.T) {
	for _, dnsResolutionEnabled := range []bool{true, false} {
		name := "dns_resolution_disabled"
		if dnsResolutionEnabled {
			name = "dns_resolution_enabled"
		}
		t.Run(name, func(t *testing.T) {
			p, handler, _ := newShortDNSTestProbe(t, dnsResolutionEnabled)

			p.handleEvent(0, shortDNSResponseRecord(undecodableDNSResponse()))

			assert.Empty(t, handler.eventTypes, "a short DNS response must not be dispatched as a regular event")
		})
	}
}

func TestShortDNSResponseUndecodableReportsFailedDNS(t *testing.T) {
	p, _, customHandler := newShortDNSTestProbe(t, true)

	packet := undecodableDNSResponse()
	record := shortDNSResponseRecord(packet)
	p.handleEvent(0, record)

	require.Equal(t, []string{events.FailedDNSRuleID}, customHandler.ruleIDs)

	// the record buffer and the probe event are both reused for the next kernel record
	for i := range record {
		record[i] = 0xee
	}
	p.handleEvent(0, shortDNSResponseRecord(validDNSResponse(t, "example.com", net.IPv4(192, 0, 2, 1))))

	data, err := customHandler.events[0].MarshalJSON()
	require.NoError(t, err)
	var failedDNS struct {
		Payload string `json:"payload"`
	}
	require.NoError(t, json.Unmarshal(data, &failedDNS))
	assert.Equal(t, base64.StdEncoding.EncodeToString(packet), failedDNS.Payload)
}

func TestShortDNSResponseDecodedFeedsResolver(t *testing.T) {
	p, handler, customHandler := newShortDNSTestProbe(t, true)

	p.handleEvent(0, shortDNSResponseRecord(validDNSResponse(t, "example.com", net.IPv4(192, 0, 2, 1))))

	assert.Equal(t, []string{"example.com"}, p.Resolvers.DNSResolver.HostListFromIP(netip.MustParseAddr("192.0.2.1")))
	assert.Empty(t, handler.eventTypes)
	assert.Empty(t, customHandler.ruleIDs)
}
