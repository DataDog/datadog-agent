// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// parity-snapshot compares the connections reported by two system-probes that
// observe the same traffic, typically two agents on one node, one running the
// kprobe connection tracer and one running fentry.
//
// Live mode reads both system-probes every -interval under its own client ID,
// compares each pair of snapshots with parity.Compare, prints a summary and
// optionally sends metrics to DogStatsD:
//
//	parity-snapshot -a /var/run/parity/kprobe/sysprobe.sock -b /var/run/parity/fentry/sysprobe.sock \
//	    -statsd unix:///var/run/datadog/dsd.socket -tags node:$NODE
//
// File mode compares two saved /network_tracer/connections responses once:
//
//	parity-snapshot -file-a kprobe.json -file-b fentry.json -v
//
// Metrics (all tagged with -tags):
//   - parity.snapshot.records{side,type,family,intra_host}: gauge, records per snapshot
//   - parity.snapshot.total{side,field,type,family,intra_host}: count, summed deltas.
//     Totals are insensitive to when each side reported a connection, so they are
//     the measure to compare over a long run.
//   - parity.snapshot.matched: count, connections reported by both sides
//   - parity.snapshot.divergences{kind,field,type,family,closed}: count. Divergences on
//     closed:false connections include timing noise from the two reads not being
//     simultaneous; closed:true ones are the reliable signal.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/parity/snapshot"
)

type options struct {
	socketA, socketB string
	fileA, fileB     string
	labelA, labelB   string
	clientID         string
	interval         time.Duration
	count            int
	maxErrors        int
	tolerance        float64
	statsdAddr       string
	tags             string
	verbose          bool
}

func main() {
	var o options
	flag.StringVar(&o.socketA, "a", "", "system-probe socket for side A")
	flag.StringVar(&o.socketB, "b", "", "system-probe socket for side B")
	flag.StringVar(&o.fileA, "file-a", "", "saved connections response for side A (file mode)")
	flag.StringVar(&o.fileB, "file-b", "", "saved connections response for side B (file mode)")
	flag.StringVar(&o.labelA, "label-a", "kprobe", "name of side A")
	flag.StringVar(&o.labelB, "label-b", "fentry", "name of side B")
	flag.StringVar(&o.clientID, "client-id", "", "system-probe client ID (default parity-snapshot-<pid>)")
	flag.DurationVar(&o.interval, "interval", 30*time.Second, "time between snapshots; keep well under system-probe's 2m client expiry")
	flag.IntVar(&o.count, "count", 0, "number of comparisons to run in live mode (0 = until interrupted)")
	flag.IntVar(&o.maxErrors, "max-errors", 0, "exit after this many consecutive failed reads, e.g. so a wrapper can find a restarted system-probe (0 = never)")
	flag.Float64Var(&o.tolerance, "tolerance", 0.02, "relative tolerance for counters and RTT of connections not closed on both sides")
	flag.StringVar(&o.statsdAddr, "statsd", "", "DogStatsD address for metrics, e.g. unix:///var/run/datadog/dsd.socket or 127.0.0.1:8125")
	flag.StringVar(&o.tags, "tags", "", "comma-separated tags added to every metric")
	flag.BoolVar(&o.verbose, "v", false, "print every divergence")
	flag.Parse()

	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "parity-snapshot:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	compareOpts := parity.Options{
		LabelA:           o.labelA,
		LabelB:           o.labelB,
		CounterTolerance: o.tolerance,
		RTTTolerance:     o.tolerance,
	}

	if o.fileA != "" || o.fileB != "" {
		ca, err := snapshot.ReadFile(o.fileA)
		if err != nil {
			return err
		}
		cb, err := snapshot.ReadFile(o.fileB)
		if err != nil {
			return err
		}
		report(o, compareOpts, nil, snapshot.ToStats(ca), snapshot.ToStats(cb))
		return nil
	}

	if o.socketA == "" || o.socketB == "" {
		return fmt.Errorf("need -a and -b (live mode) or -file-a and -file-b (file mode)")
	}
	if o.interval >= 2*time.Minute {
		return fmt.Errorf("-interval %s would let system-probe expire the client between reads", o.interval)
	}
	if o.clientID == "" {
		o.clientID = fmt.Sprintf("parity-snapshot-%d", os.Getpid())
	}

	var sd statsd.ClientInterface = &statsd.NoOpClient{}
	if o.statsdAddr != "" {
		c, err := statsd.New(o.statsdAddr, statsd.WithTags(splitTags(o.tags)))
		if err != nil {
			return fmt.Errorf("statsd: %w", err)
		}
		defer c.Close()
		sd = c
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, b := snapshot.NewClient(o.socketA), snapshot.NewClient(o.socketB)

	// the first read registers the client on both sides; its contents are
	// lifetime counters for already-active connections, so they aren't compared
	if _, _, err := snapshot.FetchPair(ctx, a, b, o.clientID); err != nil {
		return err
	}
	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	consecutiveErrors := 0
	for n := 0; o.count == 0 || n < o.count; n++ {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		ca, cb, err := snapshot.FetchPair(ctx, a, b, o.clientID)
		if err != nil {
			// a failed read leaves the two sides' client state out of step,
			// so re-register before comparing again
			fmt.Fprintln(os.Stderr, "parity-snapshot:", err)
			_ = sd.Incr("parity.snapshot.errors", nil, 1)
			if consecutiveErrors++; o.maxErrors > 0 && consecutiveErrors >= o.maxErrors {
				return fmt.Errorf("%d consecutive failed reads", consecutiveErrors)
			}
			o.clientID = fmt.Sprintf("parity-snapshot-%d-%d", os.Getpid(), n)
			if _, _, err := snapshot.FetchPair(ctx, a, b, o.clientID); err != nil {
				fmt.Fprintln(os.Stderr, "parity-snapshot: re-register:", err)
			}
			continue
		}
		consecutiveErrors = 0
		report(o, compareOpts, sd, snapshot.ToStats(ca), snapshot.ToStats(cb))
	}
	return nil
}

func report(o options, opts parity.Options, sd statsd.ClientInterface, a, b []network.ConnectionStats) {
	r := parity.Compare(a, b, opts)
	totals := map[string]map[snapshot.Class]snapshot.Totals{o.labelA: snapshot.Sum(a), o.labelB: snapshot.Sum(b)}

	fmt.Printf("%s %s=%d %s=%d matched=%d divergences=%d\n",
		time.Now().UTC().Format(time.RFC3339), o.labelA, len(a), o.labelB, len(b), r.Matched, len(r.Divergences))
	if o.verbose {
		fmt.Print(r.String())
	}
	printTotals(o, totals)

	if sd == nil {
		return
	}
	_ = sd.Count("parity.snapshot.matched", int64(r.Matched), nil, 1)
	for _, d := range r.Divergences {
		field := d.Field
		if field == "" {
			field = "none"
		}
		_ = sd.Incr("parity.snapshot.divergences", []string{
			"kind:" + kindTag(d.Kind),
			"field:" + field,
			"type:" + d.Tuple.Type.String(),
			"family:" + d.Tuple.Family.String(),
			fmt.Sprintf("closed:%t", d.Closed),
		}, 1)
	}
	for side, byClass := range totals {
		for k, t := range byClass {
			tags := []string{
				"side:" + side,
				"type:" + k.Type.String(),
				"family:" + k.Family.String(),
				fmt.Sprintf("intra_host:%t", k.IntraHost),
			}
			_ = sd.Gauge("parity.snapshot.records", float64(t.Records), tags, 1)
			for field, v := range fieldsOf(t) {
				_ = sd.Count("parity.snapshot.total", int64(v), append(tags, "field:"+field), 1)
			}
		}
	}
}

func fieldsOf(t snapshot.Totals) map[string]uint64 {
	return map[string]uint64{
		"records":         t.Records,
		"sent_bytes":      t.SentBytes,
		"recv_bytes":      t.RecvBytes,
		"sent_packets":    t.SentPackets,
		"recv_packets":    t.RecvPackets,
		"retransmits":     t.Retransmits,
		"tcp_established": t.TCPEstablished,
		"tcp_closed":      t.TCPClosed,
		"tcp_failures":    t.TCPFailures,
	}
}

func printTotals(o options, totals map[string]map[snapshot.Class]snapshot.Totals) {
	classes := make(map[snapshot.Class]bool)
	for _, byClass := range totals {
		for k := range byClass {
			classes[k] = true
		}
	}
	for k := range classes {
		ta, tb := totals[o.labelA][k], totals[o.labelB][k]
		fmt.Printf("  %s%s intra_host=%t records %d/%d closed %d/%d established %d/%d failures %d/%d bytes sent %d/%d recv %d/%d\n",
			k.Type, k.Family, k.IntraHost, ta.Records, tb.Records, ta.TCPClosed, tb.TCPClosed,
			ta.TCPEstablished, tb.TCPEstablished, ta.TCPFailures, tb.TCPFailures,
			ta.SentBytes, tb.SentBytes, ta.RecvBytes, tb.RecvBytes)
	}
}

func kindTag(k parity.Kind) string {
	switch k {
	case parity.OnlyA:
		return "only_a"
	case parity.OnlyB:
		return "only_b"
	default:
		return "field_mismatch"
	}
}

func splitTags(s string) []string {
	var tags []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
