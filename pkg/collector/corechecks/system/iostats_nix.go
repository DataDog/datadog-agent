// +build !windows

package system

import (
	"bytes"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/disk"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

/*
#include <unistd.h>
#include <sys/types.h>
#include <stdlib.h>
*/
import "C"

// For testing purpose
var ioCounters = disk.IOCounters

// kernel ticks / sec
var hz int64

const (
	// SectorSize is exported in github.com/shirou/gopsutil/disk (but not working!)
	SectorSize = 512
	kB         = (1 << 10)
)

// IOCheck doesn't need additional fields
type IOCheck struct {
	sender    aggregator.Sender
	blacklist *regexp.Regexp
}

func (c *IOCheck) nixIO() error {
	// See: https://www.xaprb.com/blog/2010/01/09/how-linux-iostat-computes-its-results/
	//      https://www.kernel.org/doc/Documentation/iostats.txt
	iomap, err := ioCounters()
	if err != nil {
		log.Errorf("system.IOCheck: could not retrieve io stats: %s", err)
		return err
	}

	//sleep and collect again
	time.Sleep(time.Second)
	iomap2, err := ioCounters()
	if err != nil {
		log.Errorf("system.IOCheck: could not retrieve io stats: %s", err)
		return err
	}

	var tagbuff bytes.Buffer
	for device, ioStats := range iomap {
		if c.blacklist != nil && c.blacklist.MatchString(device) {
			continue
		}

		ioStats2, ok := iomap2[device]
		if !ok {
			log.Infof("New device stats (possible hotplug) - full stats unavailable this iteration.")
			continue
		}

		// TODO: Different OS's might not have everything - make this OSX/Windows safe
		rs := (ioStats2.ReadCount - ioStats.ReadCount)
		ws := (ioStats2.WriteCount - ioStats.WriteCount)
		rkbs := float64(ioStats2.ReadBytes-ioStats.ReadBytes) / kB
		wkbs := float64(ioStats2.WriteBytes-ioStats.WriteBytes) / kB
		rrqms := (ioStats2.MergedReadCount - ioStats.MergedReadCount)
		wrqms := (ioStats2.MergedWriteCount - ioStats.MergedWriteCount)
		avgqusz := float64(ioStats2.WeightedIO-ioStats.WeightedIO) / 1000

		rAwait := 0.0
		wAwait := 0.0
		diffNRIO := float64(ioStats2.ReadCount - ioStats.ReadCount)
		diffNWIO := float64(ioStats2.WriteCount - ioStats.WriteCount)
		if diffNRIO != 0 {
			rAwait = float64(ioStats2.ReadTime-ioStats.ReadTime) / diffNRIO
		}
		if diffNWIO != 0 {
			wAwait = float64(ioStats2.WriteTime-ioStats.WriteTime) / diffNWIO
		}

		avgrqsz := 0.0
		aWait := 0.0
		diffNIO := diffNRIO + diffNWIO
		if diffNIO != 0 {
			avgrqsz = float64((ioStats2.ReadBytes-ioStats.ReadBytes+ioStats2.WriteBytes-ioStats.WriteBytes)/SectorSize) / diffNIO
			aWait = float64(ioStats2.ReadTime-ioStats.ReadTime+ioStats2.WriteTime-ioStats.WriteTime) / diffNIO
		}

		tput := diffNIO * float64(hz)
		util := float64(ioStats2.IoTime - ioStats.IoTime)
		svctime := 0.0
		if tput != 0 {
			svctime = util / tput
		}

		tagbuff.Reset()
		tagbuff.WriteString("device:")
		tagbuff.WriteString(device)
		tags := []string{tagbuff.String()}

		c.sender.Gauge("system.io.r_s", float64(rs), "", tags)
		c.sender.Gauge("system.io.w_s", float64(ws), "", tags)
		c.sender.Gauge("system.io.rkb_s", rkbs, "", tags)
		c.sender.Gauge("system.io.wkb_s", wkbs, "", tags)
		c.sender.Gauge("system.io.avg_rq_sz", avgrqsz, "", tags)
		c.sender.Gauge("system.io.await", aWait, "", tags)
		c.sender.Gauge("system.io.r_await", float64(rAwait), "", tags)
		c.sender.Gauge("system.io.w_await", float64(wAwait), "", tags)
		c.sender.Gauge("system.io.rrqm_s", float64(rrqms), "", tags)
		c.sender.Gauge("system.io.wrqm_s", float64(wrqms), "", tags)
		c.sender.Gauge("system.io.avg_q_sz", avgqusz, "", tags)
		if hz > 0 { // only send if we were able to collect HZ
			c.sender.Gauge("system.io.svctm", svctime, "", tags)
		}

		// Stats should be per device no device groups.
		// If device groups ever become a thing - util / 10.0 / n_devs_in_group
		// See more: (https://github.com/sysstat/sysstat/blob/v11.5.6/iostat.c#L1033-L1040)
		c.sender.Gauge("system.io.util", (util / 10.0), "", tags)

	}

	return nil
}

// Configure the IOstats check
func (c *IOCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	err := c.commonConfigure(data, initConfig)
	return err
}
// Run executes the check
func (c *IOCheck) Run() error {
	var err error
	err = c.nixIO()

	if err == nil {
		c.sender.Commit()
	}
	return err
}

func init() {
	core.RegisterCheck("io", ioFactory)
	var scClkTck C.long

	scClkTck = C.sysconf(C._SC_CLK_TCK)
	hz = int64(scClkTck)

	if hz <= 0 {
		log.Errorf("Unable to grab HZ: perhaps unavailable in your architecture" +
			"(svctm will not be available)")
	}
}

func ioFactory() check.Check {
	return &IOCheck{}
}
