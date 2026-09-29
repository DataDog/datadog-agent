// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"bytes"
	"net"
	"slices"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	// maxDNSResponseIPs is the maximum number of resolved IPs kept per DNS question. Domains behind a
	// round-robin CDN return a different answer set on every response, and DNS is the only sampled event
	// type without kernel-side deduplication, so this set would otherwise grow without bound.
	maxDNSResponseIPs = 32
	// maxDNSResponseCNames is the maximum number of CNAME targets kept per DNS question.
	maxDNSResponseCNames = 8
)

// DNSNode is used to store a DNS node
type DNSNode struct {
	NodeBase
	MatchedRules   []*model.MatchedRule
	GenerationType NodeGenerationType
	Requests       []DNSRequestNode
}

// DNSRequestNode is one question observed for a domain, together with everything that came back
// for it. A DNSNode holds at most one of these per question type.
type DNSRequestNode struct {
	Question model.DNSQuestion
	Response *DNSResponseAggregate
}

// DNSResponseAggregate is what a single DNS question resolved to, unioned across every response
// observed for it. It is deliberately not a copy of one response, and deliberately not the event
// type model.DNSResponse: only what is persisted belongs here.
type DNSResponseAggregate struct {
	IPs    []net.IPNet
	CNames []string
}

// size approximates this node's heap footprint
func (dn *DNSNode) size() int64 {
	s := int64(unsafe.Sizeof(*dn))
	s += seenBytes(dn.NodeBase)
	s += sliceBackingBytes(cap(dn.Requests), unsafe.Sizeof(DNSRequestNode{}))
	s += sliceBackingBytes(cap(dn.MatchedRules), unsafe.Sizeof((*model.MatchedRule)(nil)))
	for _, req := range dn.Requests {
		s += int64(len(req.Question.Name))
		s += req.Response.size()
	}
	return s
}

// size approximates the heap footprint of an aggregated DNS response
func (ra *DNSResponseAggregate) size() int64 {
	if ra == nil {
		return 0
	}
	s := int64(unsafe.Sizeof(*ra))
	s += sliceBackingBytes(cap(ra.IPs), unsafe.Sizeof(net.IPNet{}))
	for _, ip := range ra.IPs {
		s += int64(len(ip.IP) + len(ip.Mask))
	}
	s += sliceBackingBytes(cap(ra.CNames), unsafe.Sizeof(""))
	for _, cname := range ra.CNames {
		s += int64(len(cname))
	}
	return s
}

// newDNSResponseAggregate seeds an aggregate from the first response observed for a question.
// Events are pooled and reused by the probe, so every value is copied rather than referenced.
func newDNSResponseAggregate(resp *model.DNSResponse) *DNSResponseAggregate {
	if resp == nil {
		return nil
	}
	ra := &DNSResponseAggregate{}
	ra.merge(resp)
	return ra
}

// merge unions the IPs and CNAMEs of a newly observed response into the aggregate, up to the caps
func (ra *DNSResponseAggregate) merge(resp *model.DNSResponse) {
	for _, ip := range resp.IPs {
		if len(ra.IPs) >= maxDNSResponseIPs {
			break
		}
		if !containsIPNet(ra.IPs, ip) {
			ra.IPs = append(ra.IPs, copyIPNet(ip))
		}
	}

	for _, cname := range resp.CNames {
		if len(ra.CNames) >= maxDNSResponseCNames {
			break
		}
		if !slices.Contains(ra.CNames, cname) {
			ra.CNames = append(ra.CNames, cname)
		}
	}
}

// copyIPNet returns a deep copy of an IPNet, detaching it from the event-owned backing arrays
func copyIPNet(ipNet net.IPNet) net.IPNet {
	return net.IPNet{
		IP:   append(net.IP(nil), ipNet.IP...),
		Mask: append(net.IPMask(nil), ipNet.Mask...),
	}
}

// mergeDNSResponse folds a newly observed response into the request entry at the given index.
//
// This never reports whether anything was added: newly resolved IPs are enrichment, not profile
// drift. Answers rotate on every TTL expiry for CDN and cloud endpoints, so treating them as new
// tree data would turn ordinary DNS churn into a stream of anomaly detections.
func (dn *DNSNode) mergeDNSResponse(idx int, resp *model.DNSResponse) {
	if resp == nil {
		return
	}

	if dn.Requests[idx].Response == nil {
		dn.Requests[idx].Response = newDNSResponseAggregate(resp)
		return
	}
	dn.Requests[idx].Response.merge(resp)
}

// containsIPNet returns whether the given IPNet is already present in the list
func containsIPNet(list []net.IPNet, target net.IPNet) bool {
	for _, ipNet := range list {
		if ipNet.IP.Equal(target.IP) && bytes.Equal(ipNet.Mask, target.Mask) {
			return true
		}
	}
	return false
}

// newDNSRequestEntry builds the node-owned entry for a DNS event. Events are pooled and reused by
// the probe, so nothing may be retained by reference.
func newDNSRequestEntry(event *model.DNSEvent) DNSRequestNode {
	return DNSRequestNode{
		Question: event.Question,
		Response: newDNSResponseAggregate(event.Response),
	}
}

// NewDNSNode returns a new DNSNode instance
func NewDNSNode(event *model.DNSEvent, evt *model.Event, rules []*model.MatchedRule, generationType NodeGenerationType, imageTagID uint64) *DNSNode {
	node := &DNSNode{
		MatchedRules:   rules,
		GenerationType: generationType,
		Requests:       []DNSRequestNode{newDNSRequestEntry(event)},
	}
	node.NodeBase = NewNodeBase()
	node.AppendImageTagID(imageTagID, evt.ResolveEventTime())
	return node
}

func dnsFilterSubdomains(name string, maxDepth int) string {
	tab := strings.Split(name, ".")
	if len(tab) < maxDepth {
		return name
	}
	result := ""
	for i := 0; i < maxDepth; i++ {
		if result != "" {
			result = "." + result
		}
		result = tab[len(tab)-i-1] + result
	}
	return result
}

func (dn *DNSNode) evictImageTag(imageTagID uint64, DNSNames *utils.StringKeys) bool {
	IsNodeEmpty := dn.EvictImageTag(imageTagID)
	if IsNodeEmpty {
		return true
	}
	// reconstruct the list of all DNS requests
	if len(dn.Requests) > 0 {
		DNSNames.Insert(dn.Requests[0].Question.Name)
	}
	return false
}
