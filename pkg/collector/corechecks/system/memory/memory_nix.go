// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package memory

import (
	"bufio"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v4/mem"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// For testing purpose
var virtualMemory = mem.VirtualMemory
var swapMemory = mem.SwapMemory
var runtimeOS = runtime.GOOS
var openProcFile = func(path string) (*os.File, error) {
	return os.Open(path)
}

type memoryInstanceConfig struct {
	CollectMemoryPressure bool `yaml:"collect_memory_pressure"`
}

// Check collects memory metrics
type Check struct {
	core.CheckBase
	instanceConfig memoryInstanceConfig
}

const mbSize float64 = 1024 * 1024

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	v, errVirt := virtualMemory()
	if errVirt == nil {
		sender.Gauge("system.mem.total", float64(v.Total)/mbSize, "", nil)
		sender.Gauge("system.mem.free", float64(v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.used", float64(v.Total-v.Free)/mbSize, "", nil)
		sender.Gauge("system.mem.usable", float64(v.Available)/mbSize, "", nil)
		sender.Gauge("system.mem.pct_usable", float64(v.Available)/float64(v.Total), "", nil)

		switch runtimeOS {
		case "linux":
			e := c.linuxSpecificVirtualMemoryCheck(v)
			if e != nil {
				return e
			}
		case "freebsd":
			e := c.freebsdSpecificVirtualMemoryCheck(v)
			if e != nil {
				return e
			}
		}
	} else {
		log.Errorf("memory.Check: could not retrieve virtual memory stats: %s", errVirt)
	}

	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Gauge("system.swap.total", float64(s.Total)/mbSize, "", nil)
		sender.Gauge("system.swap.free", float64(s.Free)/mbSize, "", nil)
		sender.Gauge("system.swap.used", float64(s.Used)/mbSize, "", nil)
		sender.Gauge("system.swap.pct_free", (100-s.UsedPercent)/100, "", nil)
		sender.Rate("system.swap.swap_in", float64(s.Sin)/mbSize, "", nil)
		sender.Rate("system.swap.swap_out", float64(s.Sout)/mbSize, "", nil)
	} else {
		log.Errorf("memory.Check: could not retrieve swap memory stats: %s", errSwap)
	}

	if errVirt != nil && errSwap != nil {
		return errors.New("failed to gather any memory information")
	}

	sender.Commit()
	return nil
}

func (c *Check) linuxSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	sender.Gauge("system.mem.buffered", float64(v.Buffers)/mbSize, "", nil)
	sender.Gauge("system.mem.shared", float64(v.Shared)/mbSize, "", nil)
	sender.Gauge("system.mem.slab", float64(v.Slab)/mbSize, "", nil)
	sender.Gauge("system.mem.slab_reclaimable", float64(v.Sreclaimable)/mbSize, "", nil)
	sender.Gauge("system.mem.page_tables", float64(v.PageTables)/mbSize, "", nil)
	sender.Gauge("system.mem.commit_limit", float64(v.CommitLimit)/mbSize, "", nil)
	sender.Gauge("system.mem.committed_as", float64(v.CommittedAS)/mbSize, "", nil)
	sender.Gauge("system.swap.cached", float64(v.SwapCached)/mbSize, "", nil)

	if c.instanceConfig.CollectMemoryPressure {
		c.collectVMStatPressureMetrics(sender)
	}

	return nil
}

func (c *Check) freebsdSpecificVirtualMemoryCheck(v *mem.VirtualMemoryStat) error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.mem.cached", float64(v.Cached)/mbSize, "", nil)
	return nil
}

// Configure configures the memory check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	return yaml.Unmarshal(rawInstance, &c.instanceConfig)
}

func (c *Check) collectVMStatPressureMetrics(sender sender.Sender) {
	procfsPath := "/proc"
	if pkgconfigsetup.Datadog().IsSet("procfs_path") {
		procfsPath = pkgconfigsetup.Datadog().GetString("procfs_path")
	}

	filePath := procfsPath + "/vmstat"
	file, err := openProcFile(filePath)
	if err != nil {
		log.Debugf("memory.Check: could not open %s: %v", filePath, err)
		return
	}
	defer file.Close()

	allocstallByZone := make(map[string]uint64)
	var pgscanDirect, pgstealDirect uint64
	var pgscanKswapd, pgstealKswapd uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}

		switch {
		case strings.HasPrefix(fields[0], "allocstall_"):
			zoneName := strings.TrimPrefix(fields[0], "allocstall_")
			allocstallByZone[zoneName] = value
		case fields[0] == "pgscan_direct":
			pgscanDirect = value
		case fields[0] == "pgsteal_direct":
			pgstealDirect = value
		case fields[0] == "pgscan_kswapd":
			pgscanKswapd = value
		case fields[0] == "pgsteal_kswapd":
			pgstealKswapd = value
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debugf("memory.Check: error reading %s: %v", filePath, err)
		return
	}

	for zoneName, value := range allocstallByZone {
		sender.MonotonicCount("system.mem.allocstall", float64(value), "", []string{"zone:" + zoneName})
	}
	sender.MonotonicCount("system.mem.pgscan_direct", float64(pgscanDirect), "", nil)
	sender.MonotonicCount("system.mem.pgsteal_direct", float64(pgstealDirect), "", nil)
	sender.MonotonicCount("system.mem.pgscan_kswapd", float64(pgscanKswapd), "", nil)
	sender.MonotonicCount("system.mem.pgsteal_kswapd", float64(pgstealKswapd), "", nil)
}
