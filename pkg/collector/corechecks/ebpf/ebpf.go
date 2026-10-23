// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf contains all the ebpf-based checks.
package ebpf

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
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
	config         *EBPFCheckConfig
	sysProbeClient *sysprobeclient.CheckClient
	// previousMapEntries holds each map's entry count from the previous run,
	// keyed by series.
	previousMapEntries map[string]int64
	// previousCounts holds each counter's cumulative reading from the previous
	// run, keyed by series and metric name.
	previousCounts map[string]float64
	core.CheckBase
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &EBPFCheck{
		CheckBase:          core.NewCheckBase(CheckName),
		config:             &EBPFCheckConfig{},
		previousMapEntries: make(map[string]int64),
		previousCounts:     make(map[string]float64),
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

// Run executes the check. Every metric it gathers is written to the agent log
// as structured lines rather than submitted as a metric, one line per series.
func (m *EBPFCheck) Run() error {
	stats, err := sysprobeclient.GetCheck[ebpfcheck.EBPFStats](m.sysProbeClient, sysconfig.EBPFModule)
	if err != nil {
		if sysprobeclient.IgnoreStartupError(err) == nil {
			return nil
		}
		return fmt.Errorf("get ebpf check: %s", err)
	}

	allSeries := m.collectSeries(stats)
	if !log.ShouldLog(log.DebugLvl) {
		log.Warn("ebpf-check metrics are emitted over debug logs. Debug logging is disabled so no metrics are reported")
		return nil
	}

	now := time.Now()
	var errs []error
	for _, s := range allSeries {
		line, err := s.marshal(now)
		if err != nil {
			errs = append(errs, fmt.Errorf("marshal ebpf check series %q: %w", s.key, err))
			continue
		}
		log.Debugf("ebpf check metrics: %s", line)
	}
	return errors.Join(errs...)
}

// collectSeries turns one system-probe stats payload into the series reported
// for that run, in no particular order: one per map, one per program
// (including its kprobe stats), one per module holding the per-module totals,
// and one global series holding the overall totals.
func (m *EBPFCheck) collectSeries(stats ebpfcheck.EBPFStats) []*series {
	set := newSeriesSet()
	global := set.get(nil)

	// count records a cumulative counter as the amount it grew since the
	// previous run: the first reading is reported as-is,
	// and a reading lower than the previous one is a counter reset, so it is
	// also reported as-is. Readings not seen in this run are dropped.
	nextCounts := make(map[string]float64)
	count := func(s *series, name string, value float64) {
		key := s.key + name
		nextCounts[key] = value
		if prev, ok := m.previousCounts[key]; ok && value >= prev {
			value -= prev
		}
		s.add(name, metricTypeCount, value, nil)
	}
	defer func() { m.previousCounts = nextCounts }()

	totalMapMaxSize, totalMapRSS := uint64(0), uint64(0)
	moduleTotalMapMaxSize, moduleTotalMapRSS := make(map[string]uint64), make(map[string]uint64)
	for _, mapStats := range stats.Maps {
		totalMapMaxSize += mapStats.MaxSize
		totalMapRSS += mapStats.RSS
		if mapStats.Module == "unknown" {
			continue
		}

		s := set.get(map[string]string{
			"map_name": mapStats.Name,
			"map_type": mapStats.Type,
			"module":   mapStats.Module,
		})
		s.gauge("ebpf.maps.memory_max", float64(mapStats.MaxSize))
		if mapStats.RSS > 0 {
			s.gauge("ebpf.maps.memory_rss", float64(mapStats.RSS))
		}

		maxEntries := float64(mapStats.MaxEntries)
		var maxEntriesTags map[string]string
		switch mapStats.Type {
		case "Hash", "PerCPUHash":
			maxEntriesTags = map[string]string{"no_prealloc": strconv.FormatBool(mapStats.NoPrealloc)}
		}
		s.add("ebpf.maps.max_entries", metricTypeGauge, maxEntries, maxEntriesTags)
		if mapStats.Entries >= 0 {
			entries := float64(mapStats.Entries)
			s.gauge("ebpf.maps.entry_count", entries)
			// A map with no declared maximum has no meaningful occupation, and
			// dividing by it would yield NaN, which JSON cannot represent.
			if mapStats.MaxEntries > 0 {
				s.gauge("ebpf.maps.occupation", entries/maxEntries)
				s.gauge("ebpf.maps.occupation_increase", float64(mapStats.Entries-m.previousMapEntries[s.key])/maxEntries)
			}
			m.previousMapEntries[s.key] = mapStats.Entries
		}
		moduleTotalMapMaxSize[mapStats.Module] += mapStats.MaxSize
		moduleTotalMapRSS[mapStats.Module] += mapStats.RSS

		log.Tracef("ebpf check: map=%s maxsize=%d type=%s", mapStats.Name, mapStats.MaxSize, mapStats.Type)
	}

	if totalMapMaxSize > 0 {
		global.gauge("ebpf.maps.memory_max_total", float64(totalMapMaxSize))
	}
	if totalMapRSS > 0 {
		global.gauge("ebpf.maps.memory_rss_total", float64(totalMapRSS))
	}
	for mod, max := range moduleTotalMapMaxSize {
		if mod == "unknown" {
			continue
		}
		set.get(map[string]string{"module": mod}).gauge("ebpf.maps.memory_max_permodule_total", float64(max))
	}
	for mod, rss := range moduleTotalMapRSS {
		if mod == "unknown" {
			continue
		}
		set.get(map[string]string{"module": mod}).gauge("ebpf.maps.memory_rss_permodule_total", float64(rss))
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

		s := set.get(map[string]string{
			"program_name": progInfo.Name,
			"program_type": progInfo.Type,
			"module":       progInfo.Module,
		})
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
			s.gauge("ebpf.programs."+k, v)
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
			count(s, "ebpf.programs."+k, v)
			if log.ShouldLog(log.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}

	// Kprobe stats share their program's identity, so they land on the
	// program's series.
	for _, kprobeStatsInfo := range stats.KprobeStats {
		if kprobeStatsInfo.Module == "unknown" {
			continue
		}

		s := set.get(map[string]string{
			"program_name": kprobeStatsInfo.Name,
			"program_type": kprobeStatsInfo.Type,
			"module":       kprobeStatsInfo.Module,
		})

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
			count(s, "ebpf.kprobes."+k, v)
			if log.ShouldLog(log.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}

	if totalProgRSS > 0 {
		global.gauge("ebpf.programs.memory_rss_total", float64(totalProgRSS))
	}
	for mod, rss := range moduleTotalProgRSS {
		if mod == "unknown" {
			continue
		}
		set.get(map[string]string{"module": mod}).gauge("ebpf.programs.memory_rss_permodule_total", float64(rss))
	}
	for mod, xlatedLen := range moduleTotalXlatedLen {
		if mod == "unknown" {
			continue
		}
		if xlatedLen > 0 {
			set.get(map[string]string{"module": mod}).gauge("ebpf.programs.xlated_instruction_len_permodule_total", float64(xlatedLen))
		}
	}
	for mod, verifiedCount := range moduleTotalVerifiedCount {
		if mod == "unknown" {
			continue
		}
		if verifiedCount > 0 {
			set.get(map[string]string{"module": mod}).gauge("ebpf.programs.verified_instruction_count_permodule_total", float64(verifiedCount))
		}
	}

	return set.nonEmpty()
}
