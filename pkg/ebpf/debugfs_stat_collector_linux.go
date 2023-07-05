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

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/ebpf-manager/tracefs"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const kProbeTelemetryName = "ebpf__kprobes"

func init() {
	myPid = manager.Getpid()
}

type DebugFsStatCollector struct {
	hits   *prometheus.Desc
	misses *prometheus.Desc
}

func NewDebugFsStatCollector() *DebugFsStatCollector {
	return &DebugFsStatCollector{
		hits:   prometheus.NewDesc(kProbeTelemetryName+".hits", "Gauge tracking number of kprobe hits", nil, nil),
		misses: prometheus.NewDesc(kProbeTelemetryName+".misses", "Gauge tracking number of kprobe misses", nil, nil),
	}
}

func (collector *DebugFsStatCollector) updateProbeStats(pid int, profile string, ch chan<- prometheus.Metric) {
	if pid == 0 {
		pid = myPid
	}

	m, err := ReadKprobeProfile(profile)
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
		ch <- prometheus.MustNewConstMetric(collector.hits, prometheus.GaugeValue, float64(st.Hits), event)
		ch <- prometheus.MustNewConstMetric(collector.misses, prometheus.GaugeValue, float64(st.Misses), event)
	}
}

func (collector *DebugFsStatCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.hits
	ch <- collector.misses
}

func (collector *DebugFsStatCollector) Collect(ch chan<- prometheus.Metric) {

	root, err := tracefs.Root()
	if err != nil {
		log.Debugf("error getting tracefs root path: %s", err)
		return
	}

	collector.updateProbeStats(0, filepath.Join(root, "kprobe_profile"), ch)
	collector.updateProbeStats(0, filepath.Join(root, "uprobe_profile"), ch)
}
