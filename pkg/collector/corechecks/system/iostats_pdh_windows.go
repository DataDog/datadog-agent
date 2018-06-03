// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"bytes"
	"fmt"
	"regexp"
	"syscall"
	"unsafe"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

var (
	procGetLogicalDriveStringsW = modkernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveType            = modkernel32.NewProc("GetDriveTypeW")
)

const (
	ERROR_SUCCESS        = 0
	ERROR_FILE_NOT_FOUND = 2
	DRIVE_REMOVABLE      = 2
	DRIVE_FIXED          = 3
)

var drivelist []string

// IOCheck doesn't need additional fields
type IOCheck struct {
	core.CheckBase
	blacklist    *regexp.Regexp
	counters     map[string]*pdhutil.PdhCounterSet
	counternames map[string]string
}

func init() {
	drivebuf := make([]uint16, 256)

	// Windows API GetLogicalDriveStrings returns all of the assigned drive letters
	// https://msdn.microsoft.com/en-us/library/windows/desktop/aa364975(v=vs.85).aspx
	r, _, _ := procGetLogicalDriveStringsW.Call(
		uintptr(len(drivebuf)),
		uintptr(unsafe.Pointer(&drivebuf[0])))
	if r == 0 {
		return
	}
	drivelist = winutil.ConvertWindowsStringList(drivebuf)
}

func isDrive(instance string) bool {
	found := false
	instance += "\\"
	if instance != "C:\\" {
		return false
	}
	for _, driveletter := range drivelist {
		if instance == driveletter {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	r, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(instance))))
	if r != DRIVE_FIXED {
		return false
	}
	return true
}

// Configure the IOstats check
func (c *IOCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	err := c.commonConfigure(data, initConfig)
	if err != nil {
		return err
	}

	c.counternames = map[string]string{
		"Disk Write Bytes/sec":      "system.io.wkb_s",
		"Disk Writes/sec":           "system.io.w_s",
		"Disk Read Bytes/sec":       "system.io.rkb_s",
		"Disk Reads/sec":            "system.io.r_s",
		"Current Disk Queue Length": "system.io.avg_q_sz"}

	c.counters = make(map[string]*pdhutil.PdhCounterSet)

	for name := range c.counternames {
		c.counters[name], err = pdhutil.GetCounterSet("LogicalDisk", name, "", isDrive)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run executes the check
func (c *IOCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	var tagbuff bytes.Buffer
	for cname, cset := range c.counters {
		vals, err := cset.GetAllValues()
		if err != nil {
			fmt.Printf("Error getting values %v\n", err)
			return err
		}
		for inst, val := range vals {
			if c.blacklist != nil && c.blacklist.MatchString(inst) {
				log.Debugf("matched drive %s against blacklist; skipping", inst)
				continue
			}
			tagbuff.Reset()
			tagbuff.WriteString("device:")
			tagbuff.WriteString(inst)
			tags := []string{tagbuff.String()}
			if cname == "Disk Write Bytes/sec" || cname == "Disk Read Bytes/sec" {
				val /= 1024
			}
			sender.Gauge(c.counternames[cname], val, "", tags)
		}
	}

	sender.Commit()
	return nil
}
