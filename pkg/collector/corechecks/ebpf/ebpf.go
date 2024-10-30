// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && linux

// Package ebpf contains all the ebpf-based checks.
package ebpf

import (
	"fmt"
	"strings"

	"github.com/cihub/seelog"
	"gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ebpfcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
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
	config       *EBPFCheckConfig
	sysProbeUtil processnet.SysProbeUtil
	core.CheckBase
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &EBPFCheck{
		CheckBase: core.NewCheckBase(CheckName),
		config:    &EBPFCheckConfig{},
	}
}

// Parse parses the check configuration
func (c *EBPFCheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *EBPFCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("ebpf check config: %s", err)
	}
	if err := processnet.CheckPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")); err != nil {
		return fmt.Errorf("sysprobe socket: %s", err)
	}

	return nil
}

// Run executes the check
func (m *EBPFCheck) Run() error {
	if m.sysProbeUtil == nil {
		var err error
		m.sysProbeUtil, err = processnet.GetRemoteSystemProbeUtil(
			pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			return fmt.Errorf("sysprobe connection: %s", err)
		}
	}

	data, err := m.sysProbeUtil.GetCheck(sysconfig.EBPFModule)
	if err != nil {
		return fmt.Errorf("get ebpf check: %s", err)
	}

	sender, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	stats, ok := data.(ebpfcheck.EBPFStats)
	if !ok {
		return log.Errorf("ebpf check raw data has incorrect type: %T", stats)
	}

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
			"map_type:" + mapStats.Type.String(),
			"module:" + mapStats.Module,
		}
		sender.Gauge("ebpf.maps.memory_max", float64(mapStats.MaxSize), "", tags)
		sender.Gauge("ebpf.maps.max_entries", float64(mapStats.MaxEntries), "", tags)
		if mapStats.RSS > 0 {
			sender.Gauge("ebpf.maps.memory_rss", float64(mapStats.RSS), "", tags)
		}
		if mapStats.Entries >= 0 {
			sender.Gauge("ebpf.maps.entry_count", float64(mapStats.Entries), "", tags)
		}
		moduleTotalMapMaxSize[mapStats.Module] += mapStats.MaxSize
		moduleTotalMapRSS[mapStats.Module] += mapStats.RSS

		log.Tracef("ebpf check: map=%s maxsize=%d type=%s", mapStats.Name, mapStats.MaxSize, mapStats.Type.String())
	}

	for _, mapInfo := range stats.Maps {
		reportBaseMap(mapInfo)
	}

	if totalMapMaxSize > 0 {
		sender.Gauge("ebpf.maps.memory_max_total", float64(totalMapMaxSize), "", nil)
	}
	if totalMapRSS > 0 {
		sender.Gauge("ebpf.maps.memory_rss_total", float64(totalMapRSS), "", nil)
	}
	for mod, max := range moduleTotalMapMaxSize {
		if mod == "unknown" {
			continue
		}
		sender.Gauge("ebpf.maps.memory_max_permodule_total", float64(max), "", []string{"module:" + mod})
	}
	for mod, rss := range moduleTotalMapRSS {
		if mod == "unknown" {
			continue
		}
		sender.Gauge("ebpf.maps.memory_rss_permodule_total", float64(rss), "", []string{"module:" + mod})
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
			"program_type:" + progInfo.Type.String(),
			"module:" + progInfo.Module,
		}
		var debuglogs []string
		if log.ShouldLog(seelog.TraceLvl) {
			debuglogs = []string{"program=" + progInfo.Name, "type=" + progInfo.Type.String()}
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
			sender.Gauge("ebpf.programs."+k, v, "", tags)
			if log.ShouldLog(seelog.TraceLvl) {
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
			sender.MonotonicCountWithFlushFirstValue("ebpf.programs."+k, v, "", tags, true)
			if log.ShouldLog(seelog.TraceLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}
	if totalProgRSS > 0 {
		sender.Gauge("ebpf.programs.memory_rss_total", float64(totalProgRSS), "", nil)
	}
	for mod, rss := range moduleTotalProgRSS {
		if mod == "unknown" {
			continue
		}
		sender.Gauge("ebpf.programs.memory_rss_permodule_total", float64(rss), "", []string{"module:" + mod})
	}
	for mod, xlatedLen := range moduleTotalXlatedLen {
		if mod == "unknown" {
			continue
		}
		if xlatedLen > 0 {
			sender.Gauge("ebpf.programs.xlated_instruction_len_permodule_total", float64(xlatedLen), "", []string{"module:" + mod})
		}
	}
	for mod, verifiedCount := range moduleTotalVerifiedCount {
		if mod == "unknown" {
			continue
		}
		if verifiedCount > 0 {
			sender.Gauge("ebpf.programs.verified_instruction_count_permodule_total", float64(verifiedCount), "", []string{"module:" + mod})
		}
	}

	sender.Commit()
	return nil
}
