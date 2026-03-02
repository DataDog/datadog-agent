// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package pressure

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "pressure"
)

// For testing
var openFile = func(path string) (*os.File, error) {
	return os.Open(path)
}

// pressureStats holds the parsed total stall time from a PSI line
type pressureStats struct {
	total uint64 // cumulative microseconds of stall time
}

// Check collects PSI metrics from /proc/pressure/{cpu,memory,io}
type Check struct {
	core.CheckBase
	procPath string
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure sets up the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, initConfig, data, source)
	if err != nil {
		return err
	}

	c.procPath = "/proc"
	if pkgconfigsetup.Datadog().IsConfigured("procfs_path") {
		c.procPath = pkgconfigsetup.Datadog().GetString("procfs_path")
	}

	return nil
}

// Run executes the check
func (c *Check) Run() error {
	s, err := c.GetSender()
	if err != nil {
		return err
	}

	var anySuccess bool

	// CPU: only "some" is meaningful — "full" is always 0 per kernel design
	// (matches cgroup pattern in cgroupv2_cpu.go:49 which passes nil for fullPsi)
	if some, _, err := parsePressureFile(c.procPath + "/pressure/cpu"); err != nil {
		log.Debugf("pressure: could not read cpu pressure: %v", err)
	} else {
		if some != nil {
			s.MonotonicCount("system.pressure.cpu.some.total", float64(some.total), "", nil)
		}
		anySuccess = true
	}

	// Memory: both "some" and "full" are meaningful
	if some, full, err := parsePressureFile(c.procPath + "/pressure/memory"); err != nil {
		log.Debugf("pressure: could not read memory pressure: %v", err)
	} else {
		if some != nil {
			s.MonotonicCount("system.pressure.memory.some.total", float64(some.total), "", nil)
		}
		if full != nil {
			s.MonotonicCount("system.pressure.memory.full.total", float64(full.total), "", nil)
		}
		anySuccess = true
	}

	// IO: both "some" and "full" are meaningful
	if some, full, err := parsePressureFile(c.procPath + "/pressure/io"); err != nil {
		log.Debugf("pressure: could not read io pressure: %v", err)
	} else {
		if some != nil {
			s.MonotonicCount("system.pressure.io.some.total", float64(some.total), "", nil)
		}
		if full != nil {
			s.MonotonicCount("system.pressure.io.full.total", float64(full.total), "", nil)
		}
		anySuccess = true
	}

	if !anySuccess {
		return fmt.Errorf("pressure: could not read any PSI files from %s/pressure/", c.procPath)
	}

	s.Commit()
	return nil
}

// parsePressureFile reads a /proc/pressure/{cpu,memory,io} file and returns
// parsed stats for "some" and "full" lines. Either pointer may be nil if the
// line is not present (e.g., CPU has no "full" line on kernels < 5.13).
func parsePressureFile(path string) (some *pressureStats, full *pressureStats, err error) {
	f, err := openFile(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		total, parseErr := extractTotal(fields[1:])
		if parseErr != nil {
			log.Debugf("pressure: error parsing line in %s: %v", path, parseErr)
			continue
		}

		stats := &pressureStats{total: total}
		switch fields[0] {
		case "some":
			some = stats
		case "full":
			full = stats
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return some, full, nil
}

// extractTotal extracts the total=N value from PSI key=value fields.
// Fields are like: ["avg10=0.50", "avg60=1.20", "avg300=2.30", "total=1234567890"]
func extractTotal(fields []string) (uint64, error) {
	const prefix = "total="
	for _, field := range fields {
		if strings.HasPrefix(field, prefix) {
			return strconv.ParseUint(field[len(prefix):], 10, 64)
		}
	}
	return 0, fmt.Errorf("total field not found")
}
