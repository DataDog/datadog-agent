// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package parity compares the connections reported by two network tracers that
// observed the same traffic, e.g. the kprobe and fentry connection tracers.
//
// Connections are keyed by their ConnectionTuple. Connections that are closed on
// both sides are compared exactly, since each tracer saw the whole lifetime of
// the connection. Connections that are still active on either side depend on
// when each tracer was read, so their counters are compared with a relative
// tolerance.
//
// Neither side is assumed to be ground truth. When an Oracle is supplied (for
// example, the byte counts a test workload is known to have sent), divergences
// on the fields it knows about are attributed to the side that got them wrong.
package parity

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network"
)

// Kind classifies a Divergence
type Kind int

const (
	// OnlyA means the connection was reported by side A only
	OnlyA Kind = iota
	// OnlyB means the connection was reported by side B only
	OnlyB
	// FieldMismatch means both sides reported the connection but a field differs
	FieldMismatch
)

// Attribution records which side an Oracle says is wrong about a mismatched field
type Attribution int

const (
	// Unattributed means no Oracle value was available for the field
	Unattributed Attribution = iota
	// AWrong means side A disagrees with the Oracle and side B agrees
	AWrong
	// BWrong means side B disagrees with the Oracle and side A agrees
	BWrong
	// BothWrong means neither side agrees with the Oracle
	BothWrong
)

// Divergence describes a single difference between the two sides
type Divergence struct {
	Kind  Kind
	Tuple network.ConnectionTuple
	// Occurrence disambiguates connections that reuse the same tuple over time;
	// it is the index of the connection among those sharing Tuple, ordered by
	// LastUpdateEpoch.
	Occurrence int
	// Closed is true if the connection was closed on both sides, i.e. its
	// counters were compared exactly
	Closed bool
	// Field, A and B are only set for FieldMismatch
	Field       string
	A, B        any
	Attribution Attribution
}

// Expected holds oracle values for a connection, keyed by field name (see Fields)
type Expected map[string]uint64

// Oracle returns the known-correct counter values for a connection, if any
type Oracle func(c *network.ConnectionStats) (Expected, bool)

// Options configures a comparison
type Options struct {
	// LabelA and LabelB name the two sides in reports, e.g. "kprobe" and "fentry"
	LabelA, LabelB string
	// CounterTolerance is the relative tolerance (0.01 = 1%) applied to counters
	// of connections that are not closed on both sides. Closed connections are
	// always compared exactly.
	CounterTolerance float64
	// RTTTolerance is the relative tolerance applied to RTT and RTTVar, which are
	// sampled at different instants by each tracer
	RTTTolerance float64
	// Ignore lists field names (see Fields) to skip
	Ignore []string
	// Filter, if set, drops connections for which it returns false (on both sides)
	Filter func(c *network.ConnectionStats) bool
	// Oracle, if set, is used to attribute mismatches on the fields it knows about
	Oracle Oracle
}

// Report is the result of a comparison
type Report struct {
	LabelA, LabelB string
	// Matched is the number of connections reported by both sides
	Matched     int
	Divergences []Divergence
}

// OK returns true if no divergences were found
func (r Report) OK() bool {
	return len(r.Divergences) == 0
}

type fieldClass int

const (
	// counter fields are compared exactly for closed connections, and with
	// Options.CounterTolerance otherwise
	counter fieldClass = iota
	// rtt fields are always compared with Options.RTTTolerance
	rtt
	// exact fields are compared with ==
	exact
)

type field struct {
	name  string
	class fieldClass
	get   func(c *network.ConnectionStats) any
}

func u64[T uint16 | uint32 | uint64](v T) any { return uint64(v) }

// fields lists everything produced by the connection tracer layer. Higher-layer
// fields (IPTranslation, DNSStats, Tags, ContainerID, Via) are not compared.
var fields = []field{
	{"SentBytes", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.SentBytes) }},
	{"RecvBytes", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.RecvBytes) }},
	{"SentPackets", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.SentPackets) }},
	{"RecvPackets", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.RecvPackets) }},
	{"Retransmits", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.Retransmits) }},
	{"TCPEstablished", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPEstablished) }},
	{"TCPClosed", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPClosed) }},
	{"TCPRTOCount", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPRTOCount) }},
	{"TCPRecoveryCount", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPRecoveryCount) }},
	{"TCPReordSeen", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPReordSeen) }},
	{"TCPRcvOOOPack", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPRcvOOOPack) }},
	{"TCPDeliveredCE", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPDeliveredCE) }},
	{"TCPProbe0Count", counter, func(c *network.ConnectionStats) any { return u64(c.Monotonic.TCPProbe0Count) }},
	{"RTT", rtt, func(c *network.ConnectionStats) any { return u64(c.RTT) }},
	{"RTTVar", rtt, func(c *network.ConnectionStats) any { return u64(c.RTTVar) }},
	{"TCPFailures", exact, func(c *network.ConnectionStats) any { return failuresString(c.TCPFailures) }},
	{"ProtocolStack", exact, func(c *network.ConnectionStats) any { return c.ProtocolStack }},
	{"TLSTags", exact, func(c *network.ConnectionStats) any { return c.TLSTags }},
	{"CertInfo", exact, func(c *network.ConnectionStats) any { return c.CertInfo }},
	{"IsClosed", exact, func(c *network.ConnectionStats) any { return c.IsClosed }},
	{"IntraHost", exact, func(c *network.ConnectionStats) any { return c.IntraHost }},
	{"SPortIsEphemeral", exact, func(c *network.ConnectionStats) any { return c.SPortIsEphemeral }},
	{"TCPECNNegotiated", exact, func(c *network.ConnectionStats) any { return c.TCPECNNegotiated }},
}

// Fields returns the names of all compared fields
func Fields() []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.name)
	}
	return names
}

// failuresString renders a TCPFailures map in a stable, comparable form
func failuresString(m map[uint16]uint32) string {
	if len(m) == 0 {
		return ""
	}
	var b strings.Builder
	for _, errno := range slices.Sorted(maps.Keys(m)) {
		fmt.Fprintf(&b, "%d:%d ", errno, m[errno])
	}
	return strings.TrimSpace(b.String())
}

// Compare compares the connections reported by side A and side B.
//
// Each side should be the complete set of connections that tracer reported for
// the workload: closed connections delivered to its callback plus the active
// connections from GetConnections.
func Compare(a, b []network.ConnectionStats, opts Options) Report {
	r := Report{LabelA: opts.LabelA, LabelB: opts.LabelB}
	ignore := make(map[string]bool, len(opts.Ignore))
	for _, name := range opts.Ignore {
		ignore[name] = true
	}

	groupsA, groupsB := group(a, opts.Filter), group(b, opts.Filter)

	tuples := slices.Collect(maps.Keys(groupsA))
	for t := range groupsB {
		if _, ok := groupsA[t]; !ok {
			tuples = append(tuples, t)
		}
	}
	// deterministic report order
	slices.SortFunc(tuples, func(x, y network.ConnectionTuple) int {
		return cmp.Compare(x.String(), y.String())
	})

	for _, t := range tuples {
		ga, gb := groupsA[t], groupsB[t]
		for i := range max(len(ga), len(gb)) {
			switch {
			case i >= len(gb):
				r.Divergences = append(r.Divergences, Divergence{Kind: OnlyA, Tuple: t, Occurrence: i, Closed: ga[i].IsClosed})
			case i >= len(ga):
				r.Divergences = append(r.Divergences, Divergence{Kind: OnlyB, Tuple: t, Occurrence: i, Closed: gb[i].IsClosed})
			default:
				r.Matched++
				r.Divergences = append(r.Divergences, compareConn(ga[i], gb[i], i, ignore, opts)...)
			}
		}
	}
	return r
}

// group buckets connections by tuple, ordering each bucket by LastUpdateEpoch so
// that reuses of the same tuple pair up in the order they happened
func group(conns []network.ConnectionStats, filter func(*network.ConnectionStats) bool) map[network.ConnectionTuple][]*network.ConnectionStats {
	groups := make(map[network.ConnectionTuple][]*network.ConnectionStats)
	for i := range conns {
		c := &conns[i]
		if filter != nil && !filter(c) {
			continue
		}
		groups[c.ConnectionTuple] = append(groups[c.ConnectionTuple], c)
	}
	for _, g := range groups {
		slices.SortStableFunc(g, func(x, y *network.ConnectionStats) int {
			return cmp.Compare(x.LastUpdateEpoch, y.LastUpdateEpoch)
		})
	}
	return groups
}

func compareConn(a, b *network.ConnectionStats, occurrence int, ignore map[string]bool, opts Options) []Divergence {
	closed := a.IsClosed && b.IsClosed

	var expected Expected
	if opts.Oracle != nil {
		expected, _ = opts.Oracle(a)
	}

	var divs []Divergence
	for _, f := range fields {
		if ignore[f.name] {
			continue
		}
		va, vb := f.get(a), f.get(b)
		if fieldEqual(f.class, va, vb, closed, opts) {
			continue
		}
		d := Divergence{
			Kind:       FieldMismatch,
			Tuple:      a.ConnectionTuple,
			Occurrence: occurrence,
			Closed:     closed,
			Field:      f.name,
			A:          va,
			B:          vb,
		}
		if want, ok := expected[f.name]; ok && f.class != exact {
			d.Attribution = attribute(va.(uint64), vb.(uint64), want)
		}
		divs = append(divs, d)
	}
	return divs
}

func fieldEqual(class fieldClass, a, b any, closed bool, opts Options) bool {
	switch class {
	case counter:
		if closed {
			return a == b
		}
		return withinTolerance(a.(uint64), b.(uint64), opts.CounterTolerance)
	case rtt:
		return withinTolerance(a.(uint64), b.(uint64), opts.RTTTolerance)
	default:
		return a == b
	}
}

// withinTolerance reports whether a and b differ by at most tol relative to the
// larger of the two
func withinTolerance(a, b uint64, tol float64) bool {
	lo, hi := min(a, b), max(a, b)
	return float64(hi-lo) <= tol*float64(hi)
}

func attribute(a, b, want uint64) Attribution {
	switch aOK, bOK := a == want, b == want; {
	case aOK && !bOK:
		return BWrong
	case !aOK && bOK:
		return AWrong
	case !aOK && !bOK:
		return BothWrong
	default:
		return Unattributed
	}
}

func (r Report) label(k Kind) string {
	switch k {
	case OnlyA:
		return "only-" + cmp.Or(r.LabelA, "A")
	case OnlyB:
		return "only-" + cmp.Or(r.LabelB, "B")
	default:
		return "field-mismatch"
	}
}

func (r Report) attributionLabel(a Attribution) string {
	switch a {
	case AWrong:
		return cmp.Or(r.LabelA, "A") + "-wrong"
	case BWrong:
		return cmp.Or(r.LabelB, "B") + "-wrong"
	case BothWrong:
		return "both-wrong"
	default:
		return ""
	}
}

// String summarizes the report, listing every divergence
func (r Report) String() string {
	var b strings.Builder
	a, bl := cmp.Or(r.LabelA, "A"), cmp.Or(r.LabelB, "B")
	fmt.Fprintf(&b, "parity %s vs %s: %d matched connections, %d divergences\n", a, bl, r.Matched, len(r.Divergences))
	for _, d := range r.Divergences {
		state := "active"
		if d.Closed {
			state = "closed"
		}
		fmt.Fprintf(&b, "  %s %s#%d (%s)", r.label(d.Kind), d.Tuple, d.Occurrence, state)
		if d.Kind == FieldMismatch {
			fmt.Fprintf(&b, " %s: %s=%v %s=%v", d.Field, a, d.A, bl, d.B)
			if l := r.attributionLabel(d.Attribution); l != "" {
				fmt.Fprintf(&b, " [%s]", l)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
