// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf contains all the ebpf-based checks.
package ebpf

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	metricslogs "github.com/DataDog/datadog-agent/comp/core/metricslogs/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ebpfcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "ebpf"
)

// EBPFCheckConfig is the config of the EBPF check
type EBPFCheckConfig struct {
}

// EBPFCheck grabs eBPF map/program/perf buffer metrics
type EBPFCheck struct {
	config             *EBPFCheckConfig
	sysProbeClient     *sysprobeclient.CheckClient
	metricsLogs        metricslogs.Component
	previousMapEntries map[string]int64
	core.CheckBase
}

// metricBatch accumulates the metrics gathered during a single check run, so
// that they can be handed to the metricslogs component as one structured line.
type metricBatch struct {
	metrics []*metricslogs.Metric
}

func (b *metricBatch) add(name string, typ metricslogs.MetricType, value float64, tags []string) {
	b.metrics = append(b.metrics, &metricslogs.Metric{
		Name:  name,
		Type:  typ,
		Value: value,
		Tags:  tags,
	})
}

func (b *metricBatch) gauge(name string, value float64, tags []string) {
	b.add(name, metricslogs.MetricTypeGauge, value, tags)
}

// count reports an ever-increasing counter. The value is the counter's current
// cumulative reading, not a per-run delta: consumers of the log line compute
// deltas between successive batches themselves.
func (b *metricBatch) count(name string, value float64, tags []string) {
	b.add(name, metricslogs.MetricTypeCount, value, tags)
}

// Factory creates a new check factory
func Factory(metricsLogs metricslogs.Component) option.Option[func() check.Check] {
	if metricsLogs == nil {
		// The check reports exclusively through the metrics-as-logs forwarder,
		// so without it there is nothing it can do.
		return option.None[func() check.Check]()
	}
	return option.New(func() check.Check {
		return newCheck(metricsLogs)
	})
}

func newCheck(metricsLogs metricslogs.Component) check.Check {
	return &EBPFCheck{
		CheckBase:          core.NewCheckBase(CheckName),
		config:             &EBPFCheckConfig{},
		metricsLogs:        metricsLogs,
		previousMapEntries: make(map[string]int64),
	}
}

// Parse parses the check configuration
func (c *EBPFCheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *EBPFCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source, provider); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("ebpf check config: %s", err)
	}
	m.sysProbeClient = sysprobeclient.GetCheckClient(sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")))
	return nil
}

// Run executes the check. Every metric it gathers is reported through the
// metrics-as-logs forwarder rather than as a Datadog metric: the per-map and
// per-program tag combinations blow past metric cardinality limits.
func (m *EBPFCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[ebpfcheck.EBPFStats](m.sysProbeClient, sysconfig.EBPFModule)
	if err != nil {
		if sysprobeclient.IgnoreStartupError(err) == nil {
			return nil
		}
		return fmt.Errorf("get ebpf check: %s", err)
	}

	if err := m.metricsLogs.LogMetrics(m.collectMetrics(stats)); err != nil {
		return fmt.Errorf("log ebpf check metrics: %s", err)
	}
	return nil
}

// collectMetrics turns one system-probe stats payload into the batch of metrics
// reported for that run.
func (m *EBPFCheck) collectMetrics(stats ebpfcheck.EBPFStats) []*metricslogs.Metric {
	batch := &metricBatch{}

	totalMapMaxSize, totalMapRSS := uint64(0), uint64(0)
	moduleTotalMapMaxSize, moduleTotalMapRSS := make(map[string]uint64), make(map[string]uint64)
	reportBaseMap := func(mapStats ebpfcheck.EBPFMapStats) {
		totalMapMaxSize += mapStats.MaxSize
		totalMapRSS += mapStats.RSS
		if mapStats.Module == "unknown" {
			return
		}

		tags := []string{
			"map_name:" + mapStats.Name,
			"map_type:" + mapStats.Type,
			"module:" + mapStats.Module,
		}
		batch.gauge("ebpf.maps.memory_max", float64(mapStats.MaxSize), tags)
		if mapStats.RSS > 0 {
			batch.gauge("ebpf.maps.memory_rss", float64(mapStats.RSS), tags)
		}

		maxEntries := float64(mapStats.MaxEntries)
		batch.gauge("ebpf.maps.max_entries", maxEntries, tags)
		if mapStats.Entries >= 0 {
			entries := float64(mapStats.Entries)
			batch.gauge("ebpf.maps.entry_count", entries, tags)
			// A map with no declared maximum has no meaningful occupation, and
			// dividing by it would yield NaN, which is not representable in JSON.
			if mapStats.MaxEntries > 0 {
				batch.gauge("ebpf.maps.occupation", entries/maxEntries, tags)
				batch.gauge("ebpf.maps.occupation_increase", float64(mapStats.Entries-m.previousMapEntries[mapStats.Name])/maxEntries, tags)
			}
			m.previousMapEntries[mapStats.Name] = mapStats.Entries
		}
		moduleTotalMapMaxSize[mapStats.Module] += mapStats.MaxSize
		moduleTotalMapRSS[mapStats.Module] += mapStats.RSS

		log.Tracef("ebpf check: map=%s maxsize=%d type=%s", mapStats.Name, mapStats.MaxSize, mapStats.Type)
	}

	for _, mapInfo := range stats.Maps {
		reportBaseMap(mapInfo)
	}

	if totalMapMaxSize > 0 {
		batch.gauge("ebpf.maps.memory_max_total", float64(totalMapMaxSize), nil)
	}
	if totalMapRSS > 0 {
		batch.gauge("ebpf.maps.memory_rss_total", float64(totalMapRSS), nil)
	}
	for mod, max := range moduleTotalMapMaxSize {
		if mod == "unknown" {
			continue
		}
		batch.gauge("ebpf.maps.memory_max_permodule_total", float64(max), []string{"module:" + mod})
	}
	for mod, rss := range moduleTotalMapRSS {
		if mod == "unknown" {
			continue
		}
		batch.gauge("ebpf.maps.memory_rss_permodule_total", float64(rss), []string{"module:" + mod})
	}

	totalProgRSS := uint64(0)
	moduleTotalProgRSS := make(map[string]uint64)
	moduleTotalXlatedLen := make(map[string]uint64)
	moduleTotalVerifiedCount := make(map[string]uint64)
	for _, progInfo := range stats.Programs {
		totalProgRSS += progInfo.RSS
		if progInfo.Module == "unknown" {
			continue
		}

		tags := []string{
			"program_name:" + progInfo.Name,
			"program_type:" + progInfo.Type,
			"module:" + progInfo.Module,
		}
		var debuglogs []string
		if log.ShouldLog(log.TraceLvl) {
			debuglogs = []string{"program=" + progInfo.Name, "type=" + progInfo.Type}
		}

		gauges := map[string]float64{
			"xlated_instruction_len":     float64(progInfo.XlatedProgLen),
			"verified_instruction_count": float64(progInfo.VerifiedInsns),
			"memory_rss":                 float64(progInfo.RSS),
		}
		for k, v := range gauges {
			if v == 0 {
				continue
			}
			batch.gauge("ebpf.programs."+k, v, tags)
			if log.ShouldLog(log.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}
		moduleTotalProgRSS[progInfo.Module] += progInfo.RSS
		moduleTotalXlatedLen[progInfo.Module] += uint64(progInfo.XlatedProgLen)
		moduleTotalVerifiedCount[progInfo.Module] += uint64(progInfo.VerifiedInsns)

		monos := map[string]float64{
			"runtime_ns":       float64(progInfo.Runtime.Nanoseconds()),
			"run_count":        float64(progInfo.RunCount),
			"recursion_misses": float64(progInfo.RecursionMisses),
		}
		for k, v := range monos {
			if v == 0 {
				continue
			}
			batch.count("ebpf.programs."+k, v, tags)
			if log.ShouldLog(log.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}

	for _, kprobeStatsInfo := range stats.KprobeStats {
		if kprobeStatsInfo.Module == "unknown" {
			continue
		}

		tags := []string{
			"program_name:" + kprobeStatsInfo.Name,
			"program_type:" + kprobeStatsInfo.Type,
			"module:" + kprobeStatsInfo.Module,
		}

		var debuglogs []string
		if log.ShouldLog(log.TraceLvl) {
			debuglogs = []string{"program=" + kprobeStatsInfo.Name, "type=" + kprobeStatsInfo.Type}
		}

		monos := map[string]float64{
			"kprobe_nesting_misses":      float64(kprobeStatsInfo.KprobeMisses),
			"kretprobe_maxactive_misses": float64(kprobeStatsInfo.KretprobeMaxActiveMisses),
			"kprobe_hits":                float64(kprobeStatsInfo.KprobeHits),
		}

		for k, v := range monos {
			if v == 0 {
				continue
			}
			batch.count("ebpf.kprobes."+k, v, tags)
			if log.ShouldLog(log.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}

	if totalProgRSS > 0 {
		batch.gauge("ebpf.programs.memory_rss_total", float64(totalProgRSS), nil)
	}
	for mod, rss := range moduleTotalProgRSS {
		if mod == "unknown" {
			continue
		}
		batch.gauge("ebpf.programs.memory_rss_permodule_total", float64(rss), []string{"module:" + mod})
	}
	for mod, xlatedLen := range moduleTotalXlatedLen {
		if mod == "unknown" {
			continue
		}
		if xlatedLen > 0 {
			batch.gauge("ebpf.programs.xlated_instruction_len_permodule_total", float64(xlatedLen), []string{"module:" + mod})
		}
	}
	for mod, verifiedCount := range moduleTotalVerifiedCount {
		if mod == "unknown" {
			continue
		}
		if verifiedCount > 0 {
			batch.gauge("ebpf.programs.verified_instruction_count_permodule_total", float64(verifiedCount), []string{"module:" + mod})
		}
	}

	return batch.metrics
}
