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

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

var (
	procGetLogicalDriveStringsW = modkernel32.NewProc("GetLogicalDriveStringsW")
	procGetDriveType            = modkernel32.NewProc("GetDriveTypeW")

	driveLetterPattern    = regexp.MustCompile(`[A-Za-z]:`)
	unmountedDrivePattern = regexp.MustCompile(`HarddiskVolume([0-9])+`)
)

const (
	ERROR_SUCCESS        = 0
	ERROR_FILE_NOT_FOUND = 2
	DRIVE_REMOVABLE      = 2
	DRIVE_FIXED          = 3
)

// IOCheck doesn't need additional fields
type IOCheck struct {
	core.CheckBase
	blacklist    *regexp.Regexp
	counter      *pdhutil.PdhCounterSet
	counternames map[string]string
}

func isDrive(instance string) bool {
	if unmountedDrivePattern.MatchString(instance) {
		return true
	}
	if !driveLetterPattern.MatchString(instance) {
		return false
	}
	instance += "\\"
	r, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(instance))))
	if r != DRIVE_FIXED {
		return false
	}
	return true
}

// Configure the IOstats check
func (c *IOCheck) Configure(data integration.Data, initConfig integration.Data) error {
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

	c.counter, err = pdhutil.GetCounterSet("LogicalDisk", []string{
		"Disk Write Bytes/sec",
		"Disk Writes/sec",
		"Disk Read Bytes/sec",
		"Disk Reads/sec",
		"Current Disk Queue Length",
	}, "", isDrive)
	if err != nil {
		return err
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

	counters, err := c.counter.GetAllValues()
	if err != nil {
		fmt.Printf("Error getting values %v\n", err)
		return err
	}
	for inst, vals := range counters {
		if c.blacklist != nil && c.blacklist.MatchString(inst) {
			log.Debugf("matched drive %s against blacklist; skipping", inst)
			continue
		}
		for cname, val := range vals {
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
