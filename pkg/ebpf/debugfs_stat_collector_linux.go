// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/ebpf-manager/tracefs"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const kProbeTelemetryName = "ebpf__probes"

type DebugFsStatCollector struct {
	hits           *prometheus.Desc
	misses         *prometheus.Desc
	lastProbeStats map[string]int
}

func NewDebugFsStatCollector() *DebugFsStatCollector {
	return &DebugFsStatCollector{
		hits:           prometheus.NewDesc(kProbeTelemetryName+"__hits", "Counter tracking number of probe hits", []string{"probe_name", "probe_type"}, nil),
		misses:         prometheus.NewDesc(kProbeTelemetryName+"__misses", "Counter tracking number of probe misses", []string{"probe_name", "probe_type"}, nil),
		lastProbeStats: make(map[string]int),
	}
}

func (c *DebugFsStatCollector) updateProbeStats(pid int, probeType string, ch chan<- prometheus.Metric) {
	if pid == 0 {
		pid = myPid
	}
	root, err := tracefs.Root()
	if err != nil {
		log.Debugf("error getting tracefs root path: %s", err)
		return
	}
	profile := filepath.Join(root, probeType+"_profile")
	m, err := readKprobeProfile(profile)
	if err != nil {
		log.Debugf("error retrieving probe stats: %s", err)
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
		probeTypeKey := string(probeType[0]) + "_"
		hitsKey := "h_" + probeTypeKey + event
		missesKey := "m_" + probeTypeKey + event
		hitsDelta := float64(int(st.Hits) - c.lastProbeStats[hitsKey])
		if hitsDelta > 0 {
			ch <- prometheus.MustNewConstMetric(c.hits, prometheus.CounterValue, hitsDelta, event, probeType)
		}
		c.lastProbeStats[hitsKey] = int(st.Hits)
		missesDelta := float64(int(st.Misses) - c.lastProbeStats[missesKey])
		if missesDelta > 0 {
			ch <- prometheus.MustNewConstMetric(c.misses, prometheus.CounterValue, missesDelta, event, probeType)
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
	c.updateProbeStats(0, "kprobe", ch)
	c.updateProbeStats(0, "uprobe", ch)
}
