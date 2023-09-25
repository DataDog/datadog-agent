// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && linux

package ebpf

import (
	"fmt"
	"strings"

	"github.com/cihub/seelog"
	"gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ebpfcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck/model"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	ebpfCheckName = "ebpf"
)

// EBPFCheckConfig is the config of the EBPF check
type EBPFCheckConfig struct {
}

// EBPFCheck grabs eBPF map/program/perf buffer metrics
type EBPFCheck struct {
	config       *EBPFCheckConfig
	sysProbeUtil *processnet.RemoteSysProbeUtil
	core.CheckBase
}

// EBPFCheckFactory is exported for integration testing
func EBPFCheckFactory() check.Check {
	return &EBPFCheck{
		CheckBase: core.NewCheckBase(ebpfCheckName),
		config:    &EBPFCheckConfig{},
	}
}

func init() {
	core.RegisterCheck(ebpfCheckName, EBPFCheckFactory)
}

// Parse parses the check configuration
func (c *EBPFCheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *EBPFCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, integrationConfigDigest, initConfig, config, source); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("ebpf check config: %s", err)
	}
	if err := processnet.CheckPath(ddconfig.SystemProbe.GetString("system_probe_config.sysprobe_socket")); err != nil {
		return fmt.Errorf("sysprobe socket: %s", err)
	}

	return nil
}

// Run executes the check
func (m *EBPFCheck) Run() error {
	if m.sysProbeUtil == nil {
		var err error
		m.sysProbeUtil, err = processnet.GetRemoteSystemProbeUtil(
			ddconfig.SystemProbe.GetString("system_probe_config.sysprobe_socket"),
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
		totalMapMaxSize += mapStats.MaxSize
		totalMapRSS += mapStats.RSS
		moduleTotalMapMaxSize[mapStats.Module] += mapStats.MaxSize
		moduleTotalMapRSS[mapStats.Module] += mapStats.RSS

		log.Debugf("ebpf check: map=%s maxsize=%d type=%s", mapStats.Name, mapStats.MaxSize, mapStats.Type.String())
	}

	for _, mapInfo := range stats.Maps {
		reportBaseMap(mapInfo)
	}

	for _, pbInfo := range stats.PerfBuffers {
		reportBaseMap(pbInfo.EBPFMapStats)
		for _, cpub := range pbInfo.CPUBuffers {
			cputags := []string{
				"map_name:" + pbInfo.Name,
				"map_type:" + pbInfo.Type.String(),
				"module:" + pbInfo.Module,
				fmt.Sprintf("cpu_num:%d", cpub.CPU),
			}
			if cpub.RSS > 0 {
				sender.Gauge("ebpf.maps.memory_rss_percpu", float64(cpub.RSS), "", cputags)
			}
			if cpub.Size > 0 {
				sender.Gauge("ebpf.maps.memory_max_percpu", float64(cpub.Size), "", cputags)
			}
		}
	}

	if totalMapMaxSize > 0 {
		sender.Gauge("ebpf.maps.memory_max_total", float64(totalMapMaxSize), "", nil)
	}
	if totalMapRSS > 0 {
		sender.Gauge("ebpf.maps.memory_rss_total", float64(totalMapRSS), "", nil)
	}
	for mod, max := range moduleTotalMapMaxSize {
		sender.Gauge("ebpf.maps.memory_max_permodule_total", float64(max), "", []string{"module:" + mod})
	}
	for mod, rss := range moduleTotalMapRSS {
		sender.Gauge("ebpf.maps.memory_rss_permodule_total", float64(rss), "", []string{"module:" + mod})
	}

	totalProgRSS := uint64(0)
	moduleTotalProgRSS := make(map[string]uint64)
	for _, progInfo := range stats.Programs {
		tags := []string{
			"program_name:" + progInfo.Name,
			"program_type:" + progInfo.Type.String(),
			"module:" + progInfo.Module,
		}
		if progInfo.Tag != "" {
			tags = append(tags, "program_tag:"+progInfo.Tag)
		}
		var debuglogs []string
		if log.ShouldLog(seelog.DebugLvl) {
			debuglogs = []string{"program=" + progInfo.Name, "type=" + progInfo.Type.String()}
		}

		gauges := map[string]float64{
			"xlated_instruction_count":   float64(progInfo.XlatedProgLen),
			"verified_instruction_count": float64(progInfo.VerifiedInsns),
			"memory_rss":                 float64(progInfo.RSS),
		}
		for k, v := range gauges {
			if v == 0 {
				continue
			}
			sender.Gauge("ebpf.programs."+k, v, "", tags)
			if log.ShouldLog(seelog.DebugLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}
		totalProgRSS += progInfo.RSS
		moduleTotalProgRSS[progInfo.Module] += progInfo.RSS

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
			if log.ShouldLog(seelog.DebugLvl) {
				debuglogs = append(debuglogs, fmt.Sprintf("%s=%.0f", k, v))
			}
		}

		if log.ShouldLog(seelog.DebugLvl) {
			log.Debugf("ebpf check: %s", strings.Join(debuglogs, " "))
		}
	}
	if totalProgRSS > 0 {
		sender.Gauge("ebpf.programs.memory_rss_total", float64(totalProgRSS), "", nil)
	}
	for mod, rss := range moduleTotalProgRSS {
		sender.Gauge("ebpf.programs.memory_rss_permodule_total", float64(rss), "", []string{"module:" + mod})
	}

	sender.Commit()
	return nil
}
