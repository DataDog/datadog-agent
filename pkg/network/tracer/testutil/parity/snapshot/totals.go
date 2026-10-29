// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package snapshot

import (
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"
)

// Class groups connections for Totals
type Class struct {
	Type      model.ConnectionType
	Family    model.ConnectionFamily
	IntraHost bool
}

// Totals are the summed values of one side's connections in one Class.
//
// Counters holds plain sums keyed by field name. Breakdowns holds sums split
// by one dimension, keyed by field name and then by the dimension's value
// (see BreakdownTag for the dimension's tag key).
type Totals struct {
	Counters   map[string]uint64
	Breakdowns map[string]map[string]uint64
}

// Counter fields reported by Sum. RTT and RTT variance are reported as sums
// over the connections that carry an RTT, together with that count, so that a
// mean can be derived as rtt_sum / rtt_count.
var CounterFields = []string{
	"records",
	"sent_bytes", "recv_bytes", "sent_packets", "recv_packets",
	"retransmits", "tcp_established", "tcp_closed", "tcp_failures",
	"tcp_rto_count", "tcp_recovery_count", "tcp_reord_seen", "tcp_rcv_ooo_pack",
	"tcp_delivered_ce", "tcp_probe0_count",
	"rtt_sum", "rtt_var_sum", "rtt_count",
	"ecn_negotiated", "nat_translated",
	"dns_successes", "dns_failures", "dns_timeouts",
	"dns_success_latency_sum", "dns_failure_latency_sum",
}

// BreakdownTag maps each breakdown field to the tag key of its dimension
var BreakdownTag = map[string]string{
	"tcp_failures_by_errno":  "errno",
	"conns_by_protocol":      "protocol",
	"dns_responses_by_rcode": "rcode",
}

func newTotals() *Totals {
	t := &Totals{Counters: make(map[string]uint64), Breakdowns: make(map[string]map[string]uint64)}
	for f := range BreakdownTag {
		t.Breakdowns[f] = make(map[string]uint64)
	}
	return t
}

// Sum totals a connections payload by Class. Unlike a per-connection
// comparison, totals are not sensitive to a connection being reported in
// different snapshots on the two sides, so they can be accumulated over a long
// run.
func Sum(conns *model.Connections) map[Class]*Totals {
	out := make(map[Class]*Totals)
	for _, c := range conns.Conns {
		k := Class{Type: c.Type, Family: c.Family, IntraHost: c.IntraHost}
		t := out[k]
		if t == nil {
			t = newTotals()
			out[k] = t
		}
		addConnection(t, c)
	}
	return out
}

func addConnection(t *Totals, c *model.Connection) {
	n, b := t.Counters, t.Breakdowns
	n["records"]++
	n["sent_bytes"] += c.LastBytesSent
	n["recv_bytes"] += c.LastBytesReceived
	n["sent_packets"] += c.LastPacketsSent
	n["recv_packets"] += c.LastPacketsReceived
	n["retransmits"] += uint64(c.LastRetransmits)
	n["tcp_established"] += uint64(c.LastTcpEstablished)
	n["tcp_closed"] += uint64(c.LastTcpClosed)
	n["tcp_rto_count"] += uint64(c.LastTcpRtoCount)
	n["tcp_recovery_count"] += uint64(c.LastTcpRecoveryCount)
	n["tcp_reord_seen"] += uint64(c.LastTcpReordSeen)
	n["tcp_rcv_ooo_pack"] += uint64(c.LastTcpRcvOooPack)
	n["tcp_delivered_ce"] += uint64(c.LastTcpDeliveredCe)
	n["tcp_probe0_count"] += uint64(c.LastTcpProbe0Count)
	if c.Rtt > 0 {
		n["rtt_sum"] += uint64(c.Rtt)
		n["rtt_var_sum"] += uint64(c.RttVar)
		n["rtt_count"]++
	}
	if c.TcpEcnNegotiated {
		n["ecn_negotiated"]++
	}
	if c.IpTranslation != nil {
		n["nat_translated"]++
	}

	for errno, count := range c.TcpFailuresByErrCode {
		n["tcp_failures"] += uint64(count)
		b["tcp_failures_by_errno"][strconv.FormatUint(uint64(errno), 10)] += uint64(count)
	}

	for _, p := range c.Protocol.GetStack() {
		if p != model.ProtocolType_protocolUnknown {
			b["conns_by_protocol"][protocolName(p)]++
		}
	}

	addDNS(t, c)
}

// addDNS sums DNS stats. The agent reports them either in the top-level
// fields or, when DNS domain collection is enabled, only per domain, so both
// forms are summed (one of them is always empty).
func addDNS(t *Totals, c *model.Connection) {
	n, b := t.Counters, t.Breakdowns
	n["dns_successes"] += uint64(c.DnsSuccessfulResponses)
	n["dns_failures"] += uint64(c.DnsFailedResponses)
	n["dns_timeouts"] += uint64(c.DnsTimeouts)
	n["dns_success_latency_sum"] += c.DnsSuccessLatencySum
	n["dns_failure_latency_sum"] += c.DnsFailureLatencySum
	for rcode, count := range c.DnsCountByRcode {
		b["dns_responses_by_rcode"][strconv.FormatUint(uint64(rcode), 10)] += uint64(count)
	}

	add := func(s *model.DNSStats) {
		if s == nil {
			return
		}
		n["dns_timeouts"] += uint64(s.DnsTimeouts)
		n["dns_success_latency_sum"] += s.DnsSuccessLatencySum
		n["dns_failure_latency_sum"] += s.DnsFailureLatencySum
		for rcode, count := range s.DnsCountByRcode {
			b["dns_responses_by_rcode"][strconv.FormatUint(uint64(rcode), 10)] += uint64(count)
			if rcode == 0 {
				n["dns_successes"] += uint64(count)
			} else {
				n["dns_failures"] += uint64(count)
			}
		}
	}
	for _, s := range c.DnsStatsByDomain {
		add(s)
	}
	for _, byType := range c.DnsStatsByDomainByQueryType {
		for _, s := range byType.GetDnsStatsByQueryType() {
			add(s)
		}
	}
	for _, byType := range c.DnsStatsByDomainOffsetByQueryType {
		for _, s := range byType.GetDnsStatsByQueryType() {
			add(s)
		}
	}
}

func protocolName(p model.ProtocolType) string {
	switch p {
	case model.ProtocolType_protocolTLS:
		return "tls"
	case model.ProtocolType_protocolGRPC:
		return "grpc"
	case model.ProtocolType_protocolHTTP:
		return "http"
	case model.ProtocolType_protocolHTTP2:
		return "http2"
	case model.ProtocolType_protocolKafka:
		return "kafka"
	case model.ProtocolType_protocolMongo:
		return "mongo"
	case model.ProtocolType_protocolPostgres:
		return "postgres"
	case model.ProtocolType_protocolAMQP:
		return "amqp"
	case model.ProtocolType_protocolRedis:
		return "redis"
	case model.ProtocolType_protocolMySQL:
		return "mysql"
	default:
		return strconv.Itoa(int(p))
	}
}

// TypeName is the lowercase tag value for a connection type
func TypeName(t model.ConnectionType) string {
	if t == model.ConnectionType_udp {
		return "udp"
	}
	return "tcp"
}

// FamilyName is the lowercase tag value for a connection family
func FamilyName(f model.ConnectionFamily) string {
	if f == model.ConnectionFamily_v6 {
		return "v6"
	}
	return "v4"
}
