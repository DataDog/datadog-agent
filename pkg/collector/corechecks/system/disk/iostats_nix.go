// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package disk

import (
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem" // for system.io.block_{in,out}

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// For testing purpose
var (
	ioCounters = disk.IOCounters
	swapMemory = mem.SwapMemory

	// for test purpose
	nowNano = func() int64 { return time.Now().UnixNano() }
)

// IOCheck doesn't need additional fields
type IOCheck struct {
	core.CheckBase
	blacklist          *regexp.Regexp
	lowercaseDeviceTag bool // no effect on non-Windows platforms
	ts                 int64
	stats              map[string]disk.IOCountersStat
}

// Configure the IOstats check
func (c *IOCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.commonConfigure(integrationConfigDigest, data, initConfig, source)
	return err
}

// round a float64 with 2 decimal precision
func roundFloat(val float64) float64 {
	return math.Round(val*100) / 100
}

// We can't use values from math.MaxUint* because C size for longs changes on 32/64 bit machines
const maxULong = uint64(^uint(0)) // local max value for an uint (uint64 or uint32)
const maxULong32 = math.MaxUint32

// Compute the increment between two iostats values, taking into account they can overflow
func incrementWithOverflow(currentValue, lastValue uint64) int64 {
	ret := int64(currentValue - lastValue)
	if ret < 0 {
		// even on 64bit machines some values overflow at max32bit (like WeightedIO)
		maxValue := maxULong
		if lastValue <= maxULong32 {
			maxValue = maxULong32
		}
		// +1 is for the overflow that pushed the value from its max to 0
		ret = int64(currentValue + (maxValue - lastValue) + 1)
	}
	return ret
}

func (c *IOCheck) nixIO() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	// See: https://www.xaprb.com/blog/2010/01/09/how-linux-iostat-computes-its-results/
	//      https://www.kernel.org/doc/Documentation/iostats.txt
	iomap, err := ioCounters()
	if err != nil {
		log.Errorf("system.IOCheck: could not retrieve io stats: %s", err)
		return err
	}

	// tick in millisecond
	now := nowNano() / 1000000
	delta := float64(now - c.ts)
	deltaSecond := delta / 1000

	for device, ioStats := range iomap {
		if c.blacklist != nil && c.blacklist.MatchString(device) {
			continue
		}

		tags := []string{}
		tags = append(tags, fmt.Sprintf("device:%s", device))
		tags = append(tags, fmt.Sprintf("device_name:%s", device))

		if ioStats.Label != "" {
			tags = append(tags, fmt.Sprintf("device_label:%s", ioStats.Label))
		}

		sender.Rate("system.io.r_s", float64(ioStats.ReadCount), "", tags)
		sender.Rate("system.io.w_s", float64(ioStats.WriteCount), "", tags)
		sender.Rate("system.io.rrqm_s", float64(ioStats.MergedReadCount), "", tags)
		sender.Rate("system.io.wrqm_s", float64(ioStats.MergedWriteCount), "", tags)

		if c.ts == 0 {
			continue
		}
		lastIOStats, ok := c.stats[device]
		if !ok {
			log.Debug("New device stats (possible hotplug) - full stats unavailable this iteration.")
			continue
		}

		if delta == 0 {
			log.Debug("No delta to compute - skipping.")
			continue
		}

		// computing kB/s
		rkbs := float64(incrementWithOverflow(ioStats.ReadBytes, lastIOStats.ReadBytes)) / kB / deltaSecond
		wkbs := float64(incrementWithOverflow(ioStats.WriteBytes, lastIOStats.WriteBytes)) / kB / deltaSecond
		avgqusz := float64(incrementWithOverflow(ioStats.WeightedIO, lastIOStats.WeightedIO)) / kB / deltaSecond

		rAwait := 0.0
		wAwait := 0.0
		diffNRIO := float64(incrementWithOverflow(ioStats.ReadCount, lastIOStats.ReadCount))
		diffNWIO := float64(incrementWithOverflow(ioStats.WriteCount, lastIOStats.WriteCount))
		if diffNRIO != 0 {
			// Note we use math.MaxUint32 because this value is always 32-bit, even on 64 bit machines
			rAwait = float64(incrementWithOverflow(ioStats.ReadTime, lastIOStats.ReadTime)) / diffNRIO
		}
		if diffNWIO != 0 {
			// Note we use math.MaxUint32 because this value is always 32-bit, even on 64 bit machines
			wAwait = float64(incrementWithOverflow(ioStats.WriteTime, lastIOStats.WriteTime)) / diffNWIO
		}

		avgrqsz := 0.0
		aWait := 0.0
		diffNIO := diffNRIO + diffNWIO
		if diffNIO != 0 {
			avgrqsz = float64((incrementWithOverflow(ioStats.ReadBytes, lastIOStats.ReadBytes)+
				incrementWithOverflow(ioStats.WriteBytes, lastIOStats.WriteBytes))/SectorSize) / diffNIO
			// Note we use math.MaxUint32 because these values are always 32-bit, even on 64 bit machines
			aWait = float64(
				incrementWithOverflow(ioStats.ReadTime, lastIOStats.ReadTime)+
					incrementWithOverflow(ioStats.WriteTime, lastIOStats.WriteTime)) / diffNIO
		}

		// we are aligning ourselves with the metric reported by
		// sysstat, so itv is a time interval in 1/100th of a second
		itv := delta / 10
		tput := diffNIO * 100 / itv
		util := float64(incrementWithOverflow(ioStats.IoTime, lastIOStats.IoTime)) / itv * 100
		svctime := 0.0
		if tput != 0 {
			svctime = util / tput
		}

		sender.Gauge("system.io.rkb_s", roundFloat(rkbs), "", tags)
		sender.Gauge("system.io.wkb_s", roundFloat(wkbs), "", tags)
		sender.Gauge("system.io.avg_rq_sz", roundFloat(avgrqsz), "", tags)
		sender.Gauge("system.io.await", roundFloat(aWait), "", tags)
		sender.Gauge("system.io.r_await", roundFloat(rAwait), "", tags)
		sender.Gauge("system.io.w_await", roundFloat(wAwait), "", tags)
		sender.Gauge("system.io.avg_q_sz", roundFloat(avgqusz), "", tags)
		sender.Gauge("system.io.svctm", roundFloat(svctime), "", tags)

		// Stats should be per device no device groups.
		// If device groups ever become a thing - util / 10.0 / n_devs_in_group
		// See more: (https://github.com/sysstat/sysstat/blob/v11.5.6/iostat.c#L1033-L1040)
		sender.Gauge("system.io.util", roundFloat(util/10.0), "", tags)

	}

	// pgpgin and pgpgout are reported in /proc/vmstat and parsed by mem.swapMemory() only
	s, errSwap := swapMemory()
	if errSwap == nil {
		sender.Rate("system.io.block_in", float64(s.PgIn), "", nil)
		sender.Rate("system.io.block_out", float64(s.PgOut), "", nil)
	} else {
		log.Errorf("system.IOCheck: could not retrieve I/O block stats: %s", errSwap)
	}

	c.stats = iomap
	c.ts = now
	return nil
}

// Run executes the check
func (c *IOCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	err = c.nixIO()

	if err == nil {
		sender.Commit()
	}
	return err
}
