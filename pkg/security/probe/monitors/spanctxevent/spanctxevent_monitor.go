// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package spanctxevent holds the monitor for kernel-side per-event span
// context fill failures.
package spanctxevent

import (
	"fmt"
	"sync"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// reader identifies which span context reader attempted the fill.
type reader uint32

const (
	readerOTel reader = iota
	readerGoLabels
	readerFill
	readerLast
)

func (r reader) String() string {
	switch r {
	case readerOTel:
		return "otel_tls"
	case readerGoLabels:
		return "go_labels"
	case readerFill:
		return "fill"
	default:
		return "unknown"
	}
}

type status uint32

// Mirrors span_ctx_event_status in pkg/security/ebpf/c/include/constants/custom.h
const (
	statusOK status = iota
	statusNone
	statusNoThreadPointer
	statusReadFault
	statusTorn
	statusGNotFound
	statusAttrsReadFault
	statusMapError
	statusMalformed
	statusLast
)

// firstError mirrors SPAN_CTX_EVENT_FIRST_ERROR: statuses below it are
// outcomes, not failures, and are never counted by the kernel.
const firstError = statusNoThreadPointer

func (s status) String() string {
	switch s {
	case statusNoThreadPointer:
		return "no_thread_pointer"
	case statusReadFault:
		return "read_fault"
	case statusTorn:
		return "torn"
	case statusGNotFound:
		return "g_not_found"
	case statusAttrsReadFault:
		return "attrs_read_fault"
	case statusMapError:
		return "map_error"
	case statusMalformed:
		return "malformed"
	default:
		return "unknown"
	}
}

// kernelStats mirrors struct span_ctx_event_stats_t.
type kernelStats struct {
	Count uint64
}

// Monitor reports kernel-side per-event span context fill failures from the
// span_ctx_stats map, tagged reader: x status:.
type Monitor struct {
	statsdClient statsd.ClientInterface
	statsMap     *lib.Map
	numCPUs      int

	mu sync.Mutex
	// lastCounts is the delta baseline, keyed by the same
	// reader*statusLast+status key as the kernel map.
	lastCounts map[uint32]uint64
}

// NewMonitor returns a new Monitor.
func NewMonitor(manager *manager.Manager, statsdClient statsd.ClientInterface) (*Monitor, error) {
	statsMap, err := managerhelper.Map(manager, "span_ctx_stats")
	if err != nil {
		return nil, err
	}

	if expected := uint32(readerLast) * uint32(statusLast); statsMap.MaxEntries() != expected {
		// Would fail if the custom.h:span_ctx_event_status copy of the status type is out of sync
		return nil, fmt.Errorf("span_ctx_stats holds %d entries, expected %d: the reader/status mirrors are out of sync with custom.h", statsMap.MaxEntries(), expected)
	}

	numCPUs, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	return &Monitor{
		statsdClient: statsdClient,
		statsMap:     statsMap,
		numCPUs:      numCPUs,
		lastCounts:   make(map[uint32]uint64),
	}, nil
}

// SendStats drains the kernel span_ctx_stats map and emits the delta since
// the last drain.
func (m *Monitor) SendStats() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	perCPU := make([]kernelStats, m.numCPUs)

	for r := reader(0); r < readerLast; r++ {
		for s := firstError; s < statusLast; s++ {
			key := uint32(r)*uint32(statusLast) + uint32(s)

			if err := m.statsMap.Lookup(key, &perCPU); err != nil {
				seclog.Errorf("failed to lookup span_ctx_stats map for reader %s status %s: %s", r, s, err)
				continue
			}

			var count uint64
			for _, stat := range perCPU {
				count += stat.Count
			}

			last := m.lastCounts[key]
			if count < last {
				seclog.Errorf("span_ctx_stats count went backwards for reader %s status %s", r, s)
				continue
			}
			delta := count - last
			if delta == 0 {
				continue
			}
			m.lastCounts[key] = count

			tags := []string{"reader:" + r.String(), "status:" + s.String()}
			if err := m.statsdClient.Count(metrics.MetricSpanContextEventFailed, int64(delta), tags, 1.0); err != nil {
				return fmt.Errorf("failed to send span context event metric: %w", err)
			}
		}
	}
	return nil
}
