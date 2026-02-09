//go:build linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
)

// NetPoller reads /proc/net/snmp and /proc/net/netstat at a configurable interval
// and pushes TCP network metrics into the observer via a Handle.
//
// Designed to be detachable: single config toggle, one file, no interface changes.
// Uses the existing Handle.ObserveMetric() path — the observer doesn't know or
// care where the metrics come from.
type NetPoller struct {
	handle   observerdef.Handle
	interval time.Duration
	stopCh   chan struct{}

	// Previous counter values for rate calculation
	prev     map[string]int64
	prevTime time.Time
}

// NetPollerConfig holds configuration for the network poller.
type NetPollerConfig struct {
	Interval time.Duration // Poll interval (default: 2s)
	ProcPath string        // Override /proc path (for testing)
}

// counters we care about from /proc/net/snmp (Tcp section)
var snmpTCPCounters = map[string]string{
	"RetransSegs":      "net.tcp.retransmits",
	"InErrs":           "net.tcp.in_errors",
	"ActiveOpens":      "net.tcp.active_opens",
	"PassiveOpens":     "net.tcp.passive_opens",
	"AttemptFails":     "net.tcp.attempt_fails",
	"EstabResets":      "net.tcp.established_resets",
	"OutRsts":          "net.tcp.out_resets",
	"CurrEstab":        "net.tcp.curr_estab", // gauge, not rate
	"InSegs":           "net.tcp.in_segs",
	"OutSegs":          "net.tcp.out_segs",
}

// counters we care about from /proc/net/netstat (TcpExt section)
var netstatTCPExtCounters = map[string]string{
	"ListenOverflows": "net.tcp.listen_overflows",
	"ListenDrops":     "net.tcp.listen_drops",
	"TCPTimeouts":     "net.tcp.timeouts",
	"TCPOFOQueue":     "net.tcp.ofo_queue",        // out-of-order packets queued
	"TCPSynRetrans":   "net.tcp.syn_retransmits",
}

// gaugeMetrics are reported as absolute values, not rates
var gaugeMetrics = map[string]bool{
	"net.tcp.curr_estab": true,
}

// NewNetPoller creates a new network stats poller.
func NewNetPoller(handle observerdef.Handle, cfg NetPollerConfig) *NetPoller {
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Second
	}
	if cfg.ProcPath == "" {
		cfg.ProcPath = "/proc"
	}
	return &NetPoller{
		handle:   handle,
		interval: cfg.Interval,
		stopCh:   make(chan struct{}),
		prev:     make(map[string]int64),
	}
}

// Start begins polling in a background goroutine.
func (p *NetPoller) Start() {
	go p.run()
}

// Stop signals the poller to stop.
func (p *NetPoller) Stop() {
	close(p.stopCh)
}

func (p *NetPoller) run() {
	// Initial read to seed prev counters
	p.poll()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *NetPoller) poll() {
	now := time.Now()
	elapsed := now.Sub(p.prevTime).Seconds()

	// Read current counters
	current := make(map[string]int64)
	readProcCounters("/proc/net/snmp", "Tcp", snmpTCPCounters, current)
	readProcCounters("/proc/net/netstat", "TcpExt", netstatTCPExtCounters, current)

	// First poll: just seed, don't emit
	if p.prevTime.IsZero() || elapsed <= 0 {
		p.prev = current
		p.prevTime = now
		return
	}

	timestamp := float64(now.Unix())

	for metricName, value := range current {
		if gaugeMetrics[metricName] {
			// Gauge: emit absolute value
			p.handle.ObserveMetric(&netMetric{
				name:      metricName,
				value:     float64(value),
				timestamp: timestamp,
			})
		} else {
			// Rate: emit (current - prev) / elapsed
			prevVal, ok := p.prev[metricName]
			if !ok {
				continue
			}
			diff := value - prevVal
			if diff < 0 {
				diff = 0 // counter wrapped
			}
			rate := float64(diff) / elapsed
			p.handle.ObserveMetric(&netMetric{
				name:      metricName,
				value:     rate,
				timestamp: timestamp,
			})
		}
	}

	p.prev = current
	p.prevTime = now
}

// readProcCounters reads a /proc/net/{snmp,netstat} file and extracts
// the specified counters from the named protocol section.
//
// File format (two lines per protocol):
//   Tcp: field1 field2 field3 ...
//   Tcp: value1 value2 value3 ...
func readProcCounters(path, protocol string, wanted map[string]string, out map[string]int64) {
	f, err := os.Open(path)
	if err != nil {
		return // file doesn't exist (not linux, or restricted)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		prefix := protocol + ":"
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		// This is the header line — next line has the values
		fields := strings.Fields(line[len(prefix):])
		if !scanner.Scan() {
			return
		}
		valuesLine := scanner.Text()
		if !strings.HasPrefix(valuesLine, prefix) {
			return
		}
		values := strings.Fields(valuesLine[len(prefix):])

		if len(fields) != len(values) {
			return
		}

		for i, field := range fields {
			metricName, ok := wanted[field]
			if !ok {
				continue
			}
			val, err := strconv.ParseInt(values[i], 10, 64)
			if err != nil {
				continue
			}
			out[metricName] = val
		}
		return // found our protocol, done
	}
}

// netMetric implements observer.MetricView for network stats.
type netMetric struct {
	name      string
	value     float64
	timestamp float64
}

func (m *netMetric) GetName() string       { return m.name }
func (m *netMetric) GetValue() float64     { return m.value }
func (m *netMetric) GetRawTags() []string  { return nil }
func (m *netMetric) GetTimestamp() float64 { return m.timestamp }
func (m *netMetric) GetSampleRate() float64 { return 1.0 }

// Ensure netMetric implements MetricView at compile time.
var _ observerdef.MetricView = (*netMetric)(nil)
