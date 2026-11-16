// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"net"
	"time"

	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// newCorrelatedDNSEvent fills ev with a DNS event carrying the response and the process context of the request
func newCorrelatedDNSEvent(ev *model.Event, entry *model.ProcessCacheEntry, id uint16, question model.DNSQuestion, response *model.DNSResponse, ts time.Time, tsRaw uint64) {
	ev.Type = uint32(model.DNSEventType)
	ev.Source = model.EventSourceRelated
	ev.Timestamp = ts
	ev.TimestampRaw = tsRaw
	ev.ProcessCacheEntry = entry
	ev.ProcessContext = &entry.ProcessContext
	ev.DNS = model.DNSEvent{
		ID:       id,
		Question: question,
		Response: response,
	}
}

// correlateDNSResponseForActivityDump attributes a DNS response to the process that sent the request
// and hands it to the profile manager. It skips DispatchEvent so that rules don't match the same
// DNS request twice.
func (p *EBPFProbe) correlateDNSResponseForActivityDump(dnsLayer *layers.DNS, ips []net.IPNet, cnames []string) {
	if p.profileManager == nil || p.dnsRequests == nil {
		return
	}

	// no answer, nothing to add to the profile
	if len(ips) == 0 && len(cnames) == 0 {
		return
	}

	if len(dnsLayer.Questions) == 0 {
		return
	}
	question := dnsLayer.Questions[0]

	now := time.Now()
	// the request's spelling of the name, so that the answers land on the node it created
	entry, questionName := p.dnsRequests.MatchResponse(dnsLayer.ID, string(question.Name), uint16(question.Type), now)
	if entry == nil {
		return
	}

	ev := p.getPoolEvent()
	defer p.putBackPoolEvent(ev)

	newCorrelatedDNSEvent(ev, entry, dnsLayer.ID, model.DNSQuestion{
		Name:  questionName,
		Type:  uint16(question.Type),
		Class: uint16(question.Class),
	}, &model.DNSResponse{
		ResponseCode: uint8(dnsLayer.ResponseCode),
		IPs:          ips,
		CNames:       cnames,
	}, now, uint64(p.Resolvers.TimeResolver.ComputeMonotonicTimestamp(now)))

	// the profile manager copies what it keeps, so the event can go back to the pool
	p.profileManager.ProcessEvent(ev)
}
