// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build !android
// +build !android

package platform

import (
	"runtime"
	"strings"

	"github.com/DataDog/gohai/utils"
)

// Collect collects the Platform information.
// Returns an object which can be converted to a JSON or an error if nothing could be collected.
// Tries to collect as much information as possible.
func (platform *Platform) Collect() (result interface{}, err error) {
	result, _, err = getPlatformInfo()
	return
}

// Get returns a Platform struct already initialized, a list of warnings and an error. The method will try to collect as much
// metadata as possible, an error is returned if nothing could be collected. The list of warnings contains errors if
// some metadata could not be collected.
func Get() (*Platform, []string, error) {
	platformInfo, warnings, err := getPlatformInfo()
	if err != nil {
		return nil, nil, err
	}

	p := &Platform{}
	p.GoVersion = utils.GetString(platformInfo, "goV")
	p.GoOS = utils.GetString(platformInfo, "GOOS")
	p.GoArch = utils.GetString(platformInfo, "GOOARCH")
	p.KernelName = utils.GetString(platformInfo, "kernel_name")
	p.KernelRelease = utils.GetString(platformInfo, "kernel_release")
	p.Hostname = utils.GetString(platformInfo, "hostname")
	p.Machine = utils.GetString(platformInfo, "machine")
	p.OS = utils.GetString(platformInfo, "os")
	p.Family = utils.GetString(platformInfo, "family")
	p.KernelVersion = utils.GetString(platformInfo, "kernel_version")
	p.Processor = utils.GetString(platformInfo, "processor")
	p.HardwarePlatform = utils.GetString(platformInfo, "hardware_platform")

	return p, warnings, nil
}

func getPlatformInfo() (platformInfo map[string]string, warnings []string, err error) {

	// collect each portion, and allow the parts that succeed (even if some
	// parts fail.)  For this check, it does have the (small) liability
	// that if both the ArchInfo() and the PythonVersion() fail, the error
	// from the ArchInfo() will be lost.

	// For this, no error check.  The successful results will be added
	// to the return value, and the error stored.
	platformInfo, err = GetArchInfo()
	if platformInfo == nil {
		platformInfo = map[string]string{}
	}

	platformInfo["goV"] = strings.ReplaceAll(runtime.Version(), "go", "")
	platformInfo["GOOS"] = runtime.GOOS
	platformInfo["GOOARCH"] = runtime.GOARCH

	return
}
