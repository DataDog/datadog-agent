// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package activitytree holds activitytree related files
package activitytree

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ipv4Net builds the /32 IPNet shape the probe produces for an A record (see dnsAnswerIPNet
// in pkg/security/probe/probe_ebpf.go).
func ipv4Net(s string) net.IPNet {
	return net.IPNet{IP: net.ParseIP(s).To4(), Mask: net.CIDRMask(32, 32)}
}

// ipv6Net builds the /128 IPNet shape the probe produces for an AAAA record.
func ipv6Net(s string) net.IPNet {
	return net.IPNet{IP: net.ParseIP(s).To16(), Mask: net.CIDRMask(128, 128)}
}

// newDNSResponseEvent builds the event shape the probe hands to the tree for a DNS response:
// FullDNSResponseEventType is remapped to DNSEventType with DNS.Response populated, so from the
// tree's point of view a response is an ordinary DNS event that happens to carry answers.
func newDNSResponseEvent(name string, qtype uint16, rcode uint8, ips []net.IPNet, cnames []string) *model.Event {
	evt := newDNSEvent(name, qtype)
	evt.DNS.Response = &model.DNSResponse{
		ResponseCode: rcode,
		IPs:          ips,
		CNames:       cnames,
	}
	return evt
}

// dnsTestRoot spins up a tree with a single root process node, ready for DNS inserts.
func dnsTestRoot(t *testing.T) (*ActivityTree, *ProcessNode, uint64) {
	t.Helper()
	tree := newSizeTestTree()
	tagID := tree.GetOrInsertImageTag("v1")
	root := newSizeTestProcessNode("root")
	root.AppendImageTagID(tagID, time.Now())
	tree.ProcessNodes = []*ProcessNode{root}
	return tree, root, tagID
}

func (pn *ProcessNode) insertDNSForTest(evt *model.Event, tree *ActivityTree, tagID uint64) bool {
	return pn.InsertDNSEvent(evt, tagID, Runtime, tree.Stats, tree.DNSNames, false, 0)
}

// TestDNSResponseMergeIsNotDrift locks in the single most important property of DNS response
// enrichment: attaching resolved IPs to an already-known question must report "nothing new".
//
// In V2 the boolean returned by the insert path is the drift trigger — an insert that returns
// true on an already-sent profile dispatches the `anomaly_detection` custom rule. DNS is the one
// sampled event type with no kernel-side per-key deduplication, so every TTL rotation behind a
// CDN or cloud endpoint reaches user space. If a newly resolved IP counted as new tree data,
// ordinary DNS churn would turn into a stream of anomaly detections.
func TestDNSResponseMergeIsNotDrift(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	// The request arrives first and creates the node — this legitimately is new data.
	request := newDNSEvent("api.datadoghq.com", 1)
	require.True(t, root.insertDNSForTest(request, tree, tagID),
		"the first sighting of a domain must count as new tree data")

	// The response for that same question follows. It carries answers, but the question was
	// already known, so it must NOT be reported as new.
	response := newDNSResponseEvent("api.datadoghq.com", 1, 0, []net.IPNet{ipv4Net("34.149.115.158")}, nil)
	assert.False(t, root.insertDNSForTest(response, tree, tagID),
		"enriching a known question with its answers must not report new tree data — "+
			"the return value is the V2 drift trigger, and DNS answers rotate on every TTL expiry")

	// ...and the answers must still have been recorded.
	node := root.DNSNames["api.datadoghq.com"]
	require.NotNil(t, node)
	require.Len(t, node.Requests, 1)
	require.NotNil(t, node.Requests[0].Response, "the response was dropped instead of merged")
	require.Len(t, node.Requests[0].Response.IPs, 1)
	assert.Equal(t, "34.149.115.158", node.Requests[0].Response.IPs[0].IP.String())
}

// TestDNSResponseAccumulatesAsASet checks that answers union across responses rather than
// overwriting, and that repeats do not pile up duplicates.
func TestDNSResponseAccumulatesAsASet(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	root.insertDNSForTest(newDNSEvent("cdn.example.com", 1), tree, tagID)
	root.insertDNSForTest(newDNSResponseEvent("cdn.example.com", 1, 0,
		[]net.IPNet{ipv4Net("1.1.1.1"), ipv4Net("2.2.2.2")}, []string{"edge.example.net"}), tree, tagID)
	// Second response: one IP already seen, one new, and a duplicate CNAME.
	root.insertDNSForTest(newDNSResponseEvent("cdn.example.com", 1, 0,
		[]net.IPNet{ipv4Net("2.2.2.2"), ipv4Net("3.3.3.3")}, []string{"edge.example.net"}), tree, tagID)

	resp := root.DNSNames["cdn.example.com"].Requests[0].Response
	require.NotNil(t, resp)

	got := make([]string, 0, len(resp.IPs))
	for _, ip := range resp.IPs {
		got = append(got, ip.IP.String())
	}
	assert.ElementsMatch(t, []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}, got,
		"IPs must accumulate as a set across responses")
	assert.Equal(t, []string{"edge.example.net"}, resp.CNames, "CNAMEs must not duplicate")
}

// TestDNSResponseWithoutAnswersKeepsKnownOnes covers an answerless response — NXDOMAIN, SERVFAIL,
// a resolver blip — arriving for a question that already resolved. Because the aggregate is a
// union it must leave the known addresses alone; a future refactor that replaces rather than
// merges would silently erase them.
func TestDNSResponseWithoutAnswersKeepsKnownOnes(t *testing.T) {
	const noerror, nxdomain = uint8(0), uint8(3)
	tree, root, tagID := dnsTestRoot(t)

	root.insertDNSForTest(newDNSResponseEvent("flap.example.com", 1, noerror,
		[]net.IPNet{ipv4Net("1.2.3.4")}, []string{"edge.example.net"}), tree, tagID)
	root.insertDNSForTest(newDNSResponseEvent("flap.example.com", 1, nxdomain, nil, nil), tree, tagID)

	resp := root.DNSNames["flap.example.com"].Requests[0].Response
	require.NotNil(t, resp)
	require.Len(t, resp.IPs, 1, "an answerless response must not drop what the question resolved to")
	assert.Equal(t, "1.2.3.4", resp.IPs[0].IP.String())
	assert.Equal(t, []string{"edge.example.net"}, resp.CNames)
}

// TestDNSResponseKeepsAnswersPerQuestionType checks that A and AAAA answers stay attached to
// their own question rather than being pooled onto the domain. This is why the proto nests
// DNSResponseInfo inside DNSInfo instead of hanging flat lists off DNSNode.
func TestDNSResponseKeepsAnswersPerQuestionType(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	root.insertDNSForTest(newDNSResponseEvent("dual.example.com", 1, 0,
		[]net.IPNet{ipv4Net("9.9.9.9")}, nil), tree, tagID)
	root.insertDNSForTest(newDNSResponseEvent("dual.example.com", 28, 0,
		[]net.IPNet{ipv6Net("2606:4700::1111")}, nil), tree, tagID)

	node := root.DNSNames["dual.example.com"]
	require.Len(t, node.Requests, 2, "A and AAAA must be distinct question entries")

	byType := map[uint16]*DNSResponseAggregate{}
	for i := range node.Requests {
		byType[node.Requests[i].Question.Type] = node.Requests[i].Response
	}

	require.NotNil(t, byType[1])
	require.Len(t, byType[1].IPs, 1)
	assert.Equal(t, "9.9.9.9", byType[1].IPs[0].IP.String())

	require.NotNil(t, byType[28])
	require.Len(t, byType[28].IPs, 1)
	assert.Equal(t, "2606:4700::1111", byType[28].IPs[0].IP.String())
}

// TestDNSResponseCaps checks the per-question bounds hold. Without them a single round-robin
// domain could accumulate answers indefinitely, since kernel-side DNS sampling does not
// deduplicate and the profile persists on every tick.
func TestDNSResponseCaps(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	root.insertDNSForTest(newDNSEvent("rr.example.com", 1), tree, tagID)
	for i := 0; i < maxDNSResponseIPs*2; i++ {
		ip := ipv4Net(fmt.Sprintf("10.%d.%d.1", i/256, i%256))
		cname := fmt.Sprintf("edge-%d.example.net", i)
		root.insertDNSForTest(
			newDNSResponseEvent("rr.example.com", 1, 0, []net.IPNet{ip}, []string{cname}), tree, tagID)
	}

	resp := root.DNSNames["rr.example.com"].Requests[0].Response
	require.NotNil(t, resp)
	assert.Len(t, resp.IPs, maxDNSResponseIPs, "resolved IPs must be capped")
	assert.Len(t, resp.CNames, maxDNSResponseCNames, "CNAME targets must be capped")
}

// TestDNSResponseCapsOnFirstInsert checks the cap is also applied when the response creates the
// node, not just on the merge path — a single oversized answer set must not slip through.
func TestDNSResponseCapsOnFirstInsert(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	ips := make([]net.IPNet, 0, maxDNSResponseIPs*2)
	cnames := make([]string, 0, maxDNSResponseCNames*2)
	for i := 0; i < maxDNSResponseIPs*2; i++ {
		ips = append(ips, ipv4Net(fmt.Sprintf("10.%d.%d.1", i/256, i%256)))
		cnames = append(cnames, fmt.Sprintf("edge-%d.example.net", i))
	}

	root.insertDNSForTest(newDNSResponseEvent("big.example.com", 1, 0, ips, cnames), tree, tagID)

	resp := root.DNSNames["big.example.com"].Requests[0].Response
	require.NotNil(t, resp)
	assert.Len(t, resp.IPs, maxDNSResponseIPs)
	assert.Len(t, resp.CNames, maxDNSResponseCNames)
}

// TestDNSResponseArrivingFirst covers the case where the request was dropped upstream (rate
// limited by kernel sampling) and the response is the first thing the tree sees for that domain.
func TestDNSResponseArrivingFirst(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	assert.True(t, root.insertDNSForTest(
		newDNSResponseEvent("orphan.example.com", 1, 3, []net.IPNet{ipv4Net("5.5.5.5")}, nil), tree, tagID),
		"a domain never seen before is new tree data even when it arrives as a response")

	node := root.DNSNames["orphan.example.com"]
	require.NotNil(t, node)
	require.NotNil(t, node.Requests[0].Response)
	require.Len(t, node.Requests[0].Response.IPs, 1)
	assert.Equal(t, "5.5.5.5", node.Requests[0].Response.IPs[0].IP.String())
}

// TestDNSResponseIsNotAliasedToTheEvent guards against retaining memory owned by the event.
// The probe recycles model.Event through a sync.Pool (EBPFProbe.zeroEvent), so any slice or
// pointer stored in the tree by reference would be silently rewritten by a later event.
//
// All three insert paths must copy: creating the node, appending a new question type to an
// existing node, and merging into a known question.
func TestDNSResponseIsNotAliasedToTheEvent(t *testing.T) {
	// clobber simulates the event being recycled and reused for an unrelated packet.
	clobber := func(evt *model.Event) {
		evt.DNS.Response.IPs[0].IP[0] = 0xFF
		evt.DNS.Response.CNames[0] = "clobbered"
		evt.DNS.Response.ResponseCode = 99
		evt.DNS.Response = nil
	}

	assertIntact := func(t *testing.T, resp *DNSResponseAggregate, wantIP string) {
		t.Helper()
		require.NotNil(t, resp, "node must not hold the event's response pointer")
		require.Len(t, resp.IPs, 1)
		assert.Equal(t, wantIP, resp.IPs[0].IP.String(), "node retained the event-owned IP backing array")
		assert.Equal(t, []string{"target.example.net"}, resp.CNames)
	}

	t.Run("node creation", func(t *testing.T) {
		tree, root, tagID := dnsTestRoot(t)
		evt := newDNSResponseEvent("alias.example.com", 1, 0,
			[]net.IPNet{ipv4Net("7.7.7.7")}, []string{"target.example.net"})
		root.insertDNSForTest(evt, tree, tagID)
		clobber(evt)

		assertIntact(t, root.DNSNames["alias.example.com"].Requests[0].Response, "7.7.7.7")
	})

	t.Run("appending a new question type", func(t *testing.T) {
		tree, root, tagID := dnsTestRoot(t)
		root.insertDNSForTest(newDNSEvent("alias.example.com", 1), tree, tagID)

		// AAAA on a node that already has an A entry: takes the append path.
		evt := newDNSResponseEvent("alias.example.com", 28, 0,
			[]net.IPNet{ipv4Net("7.7.7.7")}, []string{"target.example.net"})
		require.True(t, root.insertDNSForTest(evt, tree, tagID))
		clobber(evt)

		node := root.DNSNames["alias.example.com"]
		require.Len(t, node.Requests, 2)
		assertIntact(t, node.Requests[1].Response, "7.7.7.7")
	})

	t.Run("merging into a known question", func(t *testing.T) {
		tree, root, tagID := dnsTestRoot(t)
		root.insertDNSForTest(newDNSEvent("alias.example.com", 1), tree, tagID)

		evt := newDNSResponseEvent("alias.example.com", 1, 0,
			[]net.IPNet{ipv4Net("7.7.7.7")}, []string{"target.example.net"})
		root.insertDNSForTest(evt, tree, tagID)
		clobber(evt)

		assertIntact(t, root.DNSNames["alias.example.com"].Requests[0].Response, "7.7.7.7")
	})
}

// TestSizeBytesDNSResponseChargesIncrementally mirrors TestSizeBytes_DNSAppendChargesIncrementally
// for the merge path. Merging answers grows the node, so Stats.SizeBytes must move with it —
// otherwise incremental accounting drifts from recompute and eviction over-subtracts.
func TestSizeBytesDNSResponseChargesIncrementally(t *testing.T) {
	tree, root, tagID := dnsTestRoot(t)

	root.insertDNSForTest(newDNSEvent("size.example.com", 1), tree, tagID)
	sizeBefore := tree.Stats.SizeBytes
	require.Greater(t, sizeBefore, int64(0))

	root.insertDNSForTest(newDNSResponseEvent("size.example.com", 1, 0,
		[]net.IPNet{ipv4Net("1.2.3.4"), ipv4Net("5.6.7.8")}, []string{"cdn.example.net"}), tree, tagID)
	sizeAfter := tree.Stats.SizeBytes

	assert.Greater(t, sizeAfter, sizeBefore,
		"merging DNS answers must grow Stats.SizeBytes — otherwise recompute diverges and "+
			"eviction over-subtracts, driving the metric negative")

	// Recompute is ground truth; incremental must not exceed it.
	tree.recomputeSizeBytes()
	assert.LessOrEqual(t, sizeAfter, tree.Stats.SizeBytes,
		"incremental (%d) should not exceed recompute (%d) after the merge", sizeAfter, tree.Stats.SizeBytes)
}

// TestDNSResponseProtoRoundTrip checks the wire format preserves answers, and that a question
// with no response still decodes to a nil response rather than an empty struct.
func TestDNSResponseProtoRoundTrip(t *testing.T) {
	withResponse := DNSRequestNode{
		Question: model.DNSQuestion{Name: "api.datadoghq.com", Type: 1, Class: 1, Size: 42, Count: 1},
		Response: &DNSResponseAggregate{
			IPs:    []net.IPNet{ipv4Net("34.149.115.158"), ipv6Net("2606:4700::1111")},
			CNames: []string{"cdn.example.net"},
		},
	}

	decoded := protoDecodeDNSInfo(dnsEventToProto(&withResponse))
	require.NotNil(t, decoded.Response)
	assert.Equal(t, []string{"cdn.example.net"}, decoded.Response.CNames)

	got := make([]string, 0, len(decoded.Response.IPs))
	for _, ip := range decoded.Response.IPs {
		got = append(got, ip.IP.String())
	}
	assert.Equal(t, []string{"34.149.115.158", "2606:4700::1111"}, got)

	// v4 must come back as a /32 and v6 as a /128, matching what the probe produces.
	ones, bits := decoded.Response.IPs[0].Mask.Size()
	assert.Equal(t, [2]int{32, 32}, [2]int{ones, bits})
	ones, bits = decoded.Response.IPs[1].Mask.Size()
	assert.Equal(t, [2]int{128, 128}, [2]int{ones, bits})

	// A question with no answers must not gain an empty response on the way through.
	noResponse := DNSRequestNode{Question: model.DNSQuestion{Name: "quiet.example.com", Type: 28}}
	assert.Nil(t, protoDecodeDNSInfo(dnsEventToProto(&noResponse)).Response)
}

// TestDNSResolvedIPsLabel covers the dot-graph summary: deduplicated across question types,
// bounded, and absent entirely when nothing resolved.
func TestDNSResolvedIPsLabel(t *testing.T) {
	t.Run("no answers", func(t *testing.T) {
		node := &DNSNode{Requests: []DNSRequestNode{{Question: model.DNSQuestion{Name: "a.example.com"}}}}
		assert.Empty(t, dnsResolvedIPsLabel(node))
	})

	t.Run("dedupes across question types", func(t *testing.T) {
		node := &DNSNode{Requests: []DNSRequestNode{
			{Response: &DNSResponseAggregate{IPs: []net.IPNet{ipv4Net("1.1.1.1")}}},
			{Response: &DNSResponseAggregate{IPs: []net.IPNet{ipv4Net("1.1.1.1"), ipv4Net("2.2.2.2")}}},
		}}
		assert.Equal(t, "1.1.1.1, 2.2.2.2", dnsResolvedIPsLabel(node))
	})

	t.Run("truncates", func(t *testing.T) {
		ips := make([]net.IPNet, 0, maxGraphDNSIPs+2)
		for i := 0; i < maxGraphDNSIPs+2; i++ {
			ips = append(ips, ipv4Net(fmt.Sprintf("10.0.0.%d", i+1)))
		}
		node := &DNSNode{Requests: []DNSRequestNode{{Response: &DNSResponseAggregate{IPs: ips}}}}
		assert.Equal(t, "10.0.0.1, 10.0.0.2, 10.0.0.3, ...", dnsResolvedIPsLabel(node))
	})
}

// TestDNSResponseProtoSkipsMalformedIPs checks a corrupted dump degrades gracefully: an
// unparseable address is dropped rather than failing the decode of the whole profile.
func TestDNSResponseProtoSkipsMalformedIPs(t *testing.T) {
	encoded := dnsEventToProto(&DNSRequestNode{
		Question: model.DNSQuestion{Name: "api.datadoghq.com", Type: 1},
		Response: &DNSResponseAggregate{IPs: []net.IPNet{ipv4Net("8.8.8.8")}},
	})
	encoded.Response.Ips = append(encoded.Response.Ips, "not-an-ip")

	decoded := protoDecodeDNSInfo(encoded)
	require.NotNil(t, decoded.Response)
	require.Len(t, decoded.Response.IPs, 1, "the malformed entry must be skipped, not fatal")
	assert.Equal(t, "8.8.8.8", decoded.Response.IPs[0].IP.String())
}
