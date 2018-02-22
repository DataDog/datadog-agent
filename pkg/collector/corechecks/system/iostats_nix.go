// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package system

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/xc"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/disk"
)

// For testing purpose
var ioCounters = disk.IOCounters

// kernel ticks / sec
var hz int64

// IOCheck doesn't need additional fields
type IOCheck struct {
	core.CheckBase
	blacklist *regexp.Regexp
	ts        int64
	stats     map[string]disk.IOCountersStat
}

// Configure the IOstats check
func (c *IOCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	err := c.commonConfigure(data, initConfig)
	return err
}
func (c *IOCheck) nixIO() error {
	sender, err := aggregator.GetSender(c.ID())
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

	now := time.Now().Unix()
	delta := float64(now - c.ts)

	var tagbuff bytes.Buffer
	for device, ioStats := range iomap {
		if c.blacklist != nil && c.blacklist.MatchString(device) {
			continue
		}

		tagbuff.Reset()
		tagbuff.WriteString("device:")
		tagbuff.WriteString(device)
		tags := []string{tagbuff.String()}

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

		rkbs := float64(ioStats.ReadBytes-lastIOStats.ReadBytes) / kB
		wkbs := float64(ioStats.WriteBytes-lastIOStats.WriteBytes) / kB
		avgqusz := float64(ioStats.WeightedIO-lastIOStats.WeightedIO) / 1000

		rAwait := 0.0
		wAwait := 0.0
		diffNRIO := float64(ioStats.ReadCount - lastIOStats.ReadCount)
		diffNWIO := float64(ioStats.WriteCount - lastIOStats.WriteCount)
		if diffNRIO != 0 {
			rAwait = float64(ioStats.ReadTime-lastIOStats.ReadTime) / diffNRIO
		}
		if diffNWIO != 0 {
			wAwait = float64(ioStats.WriteTime-lastIOStats.WriteTime) / diffNWIO
		}

		avgrqsz := 0.0
		aWait := 0.0
		diffNIO := diffNRIO + diffNWIO
		if diffNIO != 0 {
			avgrqsz = float64((ioStats.ReadBytes-lastIOStats.ReadBytes+ioStats.WriteBytes-lastIOStats.WriteBytes)/SectorSize) / diffNIO
			aWait = float64(ioStats.ReadTime-lastIOStats.ReadTime+ioStats.WriteTime-lastIOStats.WriteTime) / diffNIO
		}

		tput := diffNIO * float64(hz)
		util := float64(ioStats.IoTime - lastIOStats.IoTime)
		svctime := 0.0
		if tput != 0 {
			svctime = util / tput
		}

		sender.Gauge("system.io.rkb_s", rkbs/delta, "", tags)
		sender.Gauge("system.io.wkb_s", wkbs/delta, "", tags)
		sender.Gauge("system.io.avg_rq_sz", avgrqsz/delta, "", tags)
		sender.Gauge("system.io.await", aWait/delta, "", tags)
		sender.Gauge("system.io.r_await", rAwait/delta, "", tags)
		sender.Gauge("system.io.w_await", wAwait/delta, "", tags)
		sender.Gauge("system.io.avg_q_sz", avgqusz/delta, "", tags)
		if hz > 0 { // only send if we were able to collect HZ
			sender.Gauge("system.io.svctm", svctime/delta, "", tags)
		}

		// Stats should be per device no device groups.
		// If device groups ever become a thing - util / 10.0 / n_devs_in_group
		// See more: (https://github.com/sysstat/sysstat/blob/v11.5.6/iostat.c#L1033-L1040)
		sender.Gauge("system.io.util", (util / 10.0 / delta), "", tags)

	}

	c.stats = iomap
	c.ts = now
	return nil
}

// Run executes the check
func (c *IOCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	err = c.nixIO()

	if err == nil {
		sender.Commit()
	}
	return err
}

func init() {
	var err error
	hz, err = xc.GetSystemFreq()
	if err != nil {
		hz = 0
	}

	if hz <= 0 {
		log.Errorf("Unable to grab HZ: perhaps unavailable in your architecture" +
			"(svctm will not be available)")
	}
}
