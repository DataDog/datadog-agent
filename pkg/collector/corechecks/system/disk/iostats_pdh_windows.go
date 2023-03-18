// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package disk

import (
	"bytes"
	"regexp"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"

	"golang.org/x/sys/windows"
)

var (
	modkernel32 = windows.NewLazyDLL("kernel32.dll")

	procGetDriveType = modkernel32.NewProc("GetDriveTypeW")

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
	blacklist          *regexp.Regexp
	lowercaseDeviceTag bool
	pdhQuery           *pdhutil.PdhQuery
	// maps metric to counter object
	counters     map[string]pdhutil.PdhMultiInstanceCounter
	counternames map[string]string
}

var pfnGetDriveType = getDriveType

func getDriveType(drive string) uintptr {
	r, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(drive))))
	return r
}
func isDrive(instance string) bool {
	if unmountedDrivePattern.MatchString(instance) {
		return true
	}
	if !driveLetterPattern.MatchString(instance) {
		return false
	}
	instance += "\\"

	r := pfnGetDriveType(instance)
	if r != DRIVE_FIXED {
		return false
	}
	return true
}

// Configure the IOstats check
func (c *IOCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.commonConfigure(integrationConfigDigest, data, initConfig, source)
	if err != nil {
		return err
	}

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counternames = map[string]string{
		"Disk Write Bytes/sec":      "system.io.wkb_s",
		"Disk Writes/sec":           "system.io.w_s",
		"Disk Read Bytes/sec":       "system.io.rkb_s",
		"Disk Reads/sec":            "system.io.r_s",
		"Current Disk Queue Length": "system.io.avg_q_sz",
		"Avg. Disk sec/Read":        "system.io.r_await",
		"Avg. Disk sec/Write":       "system.io.w_await",
	}

	c.counters = make(map[string]pdhutil.PdhMultiInstanceCounter)
	for name := range c.counternames {
		c.counters[name] = c.pdhQuery.AddEnglishMultiInstanceCounter("LogicalDisk", name, isDrive)
	}

	return nil
}

// Run executes the check
func (c *IOCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Fetch PDH query values
	err = c.pdhQuery.CollectQueryData()
	if err == nil {
		// Get values for PDH counters
		var tagbuff bytes.Buffer
		for cname, counter := range c.counters {
			if counter == nil {
				// counter is not yet initialized
				continue
			}
			// get counter values
			vals, err := counter.GetAllValues()
			if err != nil {
				c.Warnf("io.Check: Error getting values for %v: %v", cname, err)
				continue
			}
			for inst, val := range vals {
				if c.blacklist != nil && c.blacklist.MatchString(inst) {
					log.Debugf("matched drive %s against blacklist; skipping", inst)
					continue
				}
				tagbuff.Reset()
				tagbuff.WriteString("device:")
				if c.lowercaseDeviceTag {
					inst = strings.ToLower(inst)
				}
				tagbuff.WriteString(inst)
				tags := []string{tagbuff.String()}

				if !driveLetterPattern.MatchString(inst) {
					// if this is not a drive letter, add device_name to tags
					tags = append(tags, "device_name:"+inst)
				}

				if cname == "Disk Write Bytes/sec" || cname == "Disk Read Bytes/sec" {
					val /= kB
				}
				if cname == "Avg. Disk sec/Read" || cname == "Avg. Disk sec/Write" {
					// r_await/w_await are in milliseconds, but the performance counter
					// is (obviously) in seconds.  Normalize:
					val *= 1000
				}

				sender.Gauge(c.counternames[cname], val, "", tags)
			}
		}
	} else {
		c.Warnf("file_handle.Check: Could not collect performance counter data: %v", err)
	}

	sender.Commit()
	return nil
}
