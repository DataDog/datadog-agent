// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const kprobeTelemetryName = "ebpf__probes"

type profileType byte

const (
	kprobe profileType = iota
	uprobe
)

func (p profileType) String() string {
	switch p {
	case kprobe:
		return "kprobe"
	case uprobe:
		return "uprobe"
	default:
		return ""
	}
}

func (p profileType) path() string {
	switch p {
	case kprobe:
		return "kprobe_profile"
	case uprobe:
		return "uprobe_profile"
	default:
		return ""
	}
}

type counterType byte

const (
	hits counterType = iota
	misses
)

type eventKey struct {
	profile profileType
	counter counterType
	event   string
}

// DebugFsStatCollector implements the prometheus Collector interface
// for collecting statistics about kprobe/uprobe hits/misses from debugfs/tracefs.
type DebugFsStatCollector struct {
	sync.Mutex
	hits           *prometheus.Desc
	misses         *prometheus.Desc
	lastProbeStats map[eventKey]int
	tracefsRoot    string
}

// NewDebugFsStatCollector creates a DebugFsStatCollector
func NewDebugFsStatCollector() prometheus.Collector {
	root, err := tracefs.Root()
	if err != nil {
		log.Debugf("error getting tracefs root path: %s", err)
		return &NoopDebugFsStatCollector{}
	}
	return &DebugFsStatCollector{
		hits:           prometheus.NewDesc(kprobeTelemetryName+"__hits", "Counter tracking number of probe hits", []string{"probe_name", "probe_type"}, nil),
		misses:         prometheus.NewDesc(kprobeTelemetryName+"__misses", "Counter tracking number of probe misses", []string{"probe_name", "probe_type"}, nil),
		lastProbeStats: make(map[eventKey]int),
		tracefsRoot:    root,
	}
}

func (c *DebugFsStatCollector) updateProbeStats(pid int, probeType profileType, ch chan<- prometheus.Metric) {
	profile := filepath.Join(c.tracefsRoot, probeType.path())
	m, err := readKprobeProfile(profile)
	if err != nil {
		log.Debugf("error retrieving %s probe stats: %s", probeType.String(), err)
		return
	}
	for event, st := range m {
		parts := eventRegexp.FindStringSubmatch(event)
		if len(parts) > 2 {
			// only get stats for our pid
			if len(parts) > 3 {
				parsePid, err := strconv.ParseInt(parts[3], 10, 32)
				if err != nil || int(parsePid) != pid {
					continue
				}
			}
			// strip UID and PID from name
			event = parts[1]
		}
		event = strings.ToLower(event)

		hitsKey := eventKey{probeType, hits, event}
		hitsDelta := float64(int(st.Hits) - c.lastProbeStats[hitsKey])
		if hitsDelta > 0 {
			ch <- prometheus.MustNewConstMetric(c.hits, prometheus.CounterValue, hitsDelta, event, probeType.String())
		}
		c.lastProbeStats[hitsKey] = int(st.Hits)

		missesKey := eventKey{probeType, misses, event}
		missesDelta := float64(int(st.Misses) - c.lastProbeStats[missesKey])
		if missesDelta > 0 {
			ch <- prometheus.MustNewConstMetric(c.misses, prometheus.CounterValue, missesDelta, event, probeType.String())
		}
		c.lastProbeStats[missesKey] = int(st.Misses)
	}
}

// Describe returns all descriptions of the collector
func (c *DebugFsStatCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.hits
	ch <- c.misses
}

// Collect returns the current state of all metrics of the collector
func (c *DebugFsStatCollector) Collect(ch chan<- prometheus.Metric) {
	c.Lock()
	defer c.Unlock()
	c.updateProbeStats(myPid, kprobe, ch)
	c.updateProbeStats(myPid, uprobe, ch)
}
