package system

import (
	"bytes"
	"fmt"
	"regexp"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/dd-trace-go/tracer"
	"github.com/StackExchange/wmi"
	log "github.com/cihub/seelog"
)

func (c *WMIIOCheck) String() string {
	return "WMIIOCheckWMI"
}

// Configure the IOstats check
func (c *WMIIOCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	err := error(nil)

	blacklistRe := config.Datadog.GetString("device_blacklist_re")
	if blacklistRe != "" {
		c.blacklist, err = regexp.Compile(blacklistRe)
	}
	return err
}

// InitSender initializes a sender
func (c *WMIIOCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *WMIIOCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *WMIIOCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *WMIIOCheck) Stop() {}

//type Win32_PerfRawData_PerfDisk_LogicalDisk struct {
type Win32_PerfRawData_PerfDisk_LogicalDisk struct {
	CurrentDiskQueueLength uint32
	DiskReadBytesPerSec    uint64
	DiskReadsPerSec        uint32
	DiskWriteBytesPerSec   uint64
	DiskWritesPerSec       uint32
	Frequency_Sys100NS     uint64
	Name                   string
	Timestamp_Sys100NS     uint64
}

// WMIIOCheck doesn't need additional fields
type WMIIOCheck struct {
	sender    aggregator.Sender
	blacklist *regexp.Regexp
	drivemap  map[string]Win32_PerfRawData_PerfDisk_LogicalDisk
}

func init() {
	core.RegisterCheck("io_wmi", wmiioFactory)
}

func wmiioFactory() check.Check {
	log.Infof("WMIIOCheck factory")
	c := &WMIIOCheck{}
	c.drivemap = make(map[string]Win32_PerfRawData_PerfDisk_LogicalDisk, 0)

	drivebuf := make([]byte, 256)

	r, _, _ := ProcGetLogicalDriveStringsW.Call(
		uintptr(len(drivebuf)),
		uintptr(unsafe.Pointer(&drivebuf[0])))
	if r == 0 {
		log.Errorf("IO Factory failed to get drive strings")
		return nil
	}
	for _, v := range drivebuf {
		// between 'A' & 'Z'
		if v >= 65 && v <= 90 {
			drive := string(v)
			r, _, _ = ProcGetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(drive + `:\`))))
			if r != DRIVE_FIXED {
				continue
			}
			c.drivemap[drive] = Win32_PerfRawData_PerfDisk_LogicalDisk{}
		}
	}
	log.Infof("IO Factory -- success")
	return c
}

//
func computeValue(pvs Win32_PerfRawData_PerfDisk_LogicalDisk, cur *Win32_PerfRawData_PerfDisk_LogicalDisk) (ret map[string]float64, e error) {

	e = nil
	ret = make(map[string]float64, 0)
	var f uint64 = pvs.Frequency_Sys100NS
	var dt uint64 = cur.Timestamp_Sys100NS - pvs.Timestamp_Sys100NS
	log.Infof("DeltaT is %d (%d)", dt/10000000, dt)

	if f == 0 {
		log.Errorf("Frequency is zero?")
		return nil, fmt.Errorf("Divide by zero (frequency)")
	}
	if dt == 0 {
		log.Errorf("delta-T is zero?")
		return nil, fmt.Errorf("Divide by zero (delta-T)")
	}

	v := (cur.DiskWriteBytesPerSec - pvs.DiskWriteBytesPerSec) / (dt / f)
	ret["system.wmi.io.wkb_s"] = float64(v / 1024)

	v = (uint64(cur.DiskWritesPerSec) - uint64(pvs.DiskWritesPerSec)) / (dt / f)
	ret["system.wmi.io.w_s"] = float64(v)

	v = (cur.DiskReadBytesPerSec - pvs.DiskReadBytesPerSec) / (dt / f)
	ret["system.wmi.io.rkb_s"] = float64(v / 1024)

	v = (uint64(cur.DiskReadsPerSec) - uint64(pvs.DiskReadsPerSec)) / (dt / f)
	ret["system.wmi.io.r_s"] = float64(v)

	v = (uint64(cur.CurrentDiskQueueLength) - uint64(pvs.CurrentDiskQueueLength)) / (dt / f)
	ret["system.wmi.io.avg_q_sz"] = float64(v)

	return ret, e

}

// Run executes the check
func (c *WMIIOCheck) Run() error {
	span := tracer.NewRootSpan("ddagent.check.system.io", "wmi-io", "Run")
	defer span.Finish()
	log.Infof("Running IO Check")
	var dst []Win32_PerfRawData_PerfDisk_LogicalDisk
	start := time.Now()
	err := wmi.Query("SELECT Name, DiskWriteBytesPerSec, DiskWritesPerSec, DiskReadBytesPerSec, DiskReadsPerSec, CurrentDiskQueueLength, Timestamp_Sys100NS, Frequency_Sys100NS FROM Win32_PerfRawData_PerfDisk_LogicalDisk ", &dst)
	if err != nil {
		log.Errorf("Error in WMI query %s", err.Error())
		return err
	}
	var tagbuff bytes.Buffer
	for _, d := range dst {
		log.Debugf("Got drive %s", d.Name)
		if len(d.Name) > 3 {
			continue
		}
		drive := d.Name
		log.Infof("checking drive %s against blacklist", drive)
		if c.blacklist != nil && c.blacklist.MatchString(drive) {
			continue
		}

		tagbuff.Reset()
		tagbuff.WriteString("device:")
		tagbuff.WriteString(drive)
		tags := []string{tagbuff.String()}
		log.Infof("Created tags %s", tags)
		if len(c.drivemap[d.Name].Name) != 0 {
			// have a previous value we can compute from
			metrics, err := computeValue(c.drivemap[d.Name], &d)
			if err != nil {
				log.Errorf("Error computing WMI statistics: %s", err)
			} else {
				log.Infof("returned metrics %d", len(metrics))
				for k, v := range metrics {
					log.Infof("Setting %s to %f", k, v)
					c.sender.Gauge(k, v, "", tags)
				}
			}

		} else {
			log.Infof("Length of drive was %d", len(c.drivemap[d.Name].Name))
		}
		log.Infof("Setting previous values")
		c.drivemap[d.Name] = d
	}
	c.sender.Commit()
	log.Infof("Elapsed time: %s", time.Since(start).String())
	return nil
}
