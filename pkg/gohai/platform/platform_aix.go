// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package platform

import (
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	gopsutilhost "github.com/shirou/gopsutil/v4/host"
	"golang.org/x/sys/unix"
)

func (info *Info) fillPlatformInfo() {
	info.Family = utils.NewErrorValue[string](utils.ErrNotCollectable)
	info.OS = utils.NewValue("AIX")
	// On AIX, runtime.GOARCH is "ppc64" which is more useful than uname.Machine
	// (which returns the hardware serial number, e.g. "00F9D80F4C00").
	info.Processor = utils.NewValue(runtime.GOARCH)
	info.HardwarePlatform = utils.NewValue(runtime.GOARCH)

	// uname provides KernelRelease (AIX major version, e.g. "7") and Machine
	// (hardware serial). These are not available from gopsutil.
	// uname is stack-allocated so its fields are always safe to read even on error.
	var uname unix.Utsname
	unameErr := unix.Uname(&uname)
	info.KernelRelease = utils.NewValueFrom(utils.StringFromBytes(uname.Release[:]), unameErr)
	info.Machine = utils.NewValueFrom(utils.StringFromBytes(uname.Machine[:]), unameErr)

	// gopsutil provides the full AIX maintenance level as KernelVersion
	// (e.g. "7300-02-02-2419") and hostname, which are more informative.
	hostInfo, err := gopsutilhost.Info()
	if err == nil {
		info.KernelName = utils.NewValue("AIX")
		info.Hostname = utils.NewValue(hostInfo.Hostname)
		info.KernelVersion = utils.NewValue(hostInfo.KernelVersion)
	} else if unameErr == nil {
		// Fall back to uname fields if gopsutil fails.
		info.KernelName = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
		info.Hostname = utils.NewValue(utils.StringFromBytes(uname.Nodename[:]))
		info.KernelVersion = utils.NewValue(utils.StringFromBytes(uname.Version[:]))
	} else {
		// Both gopsutil and uname failed; report the actual errors.
		info.KernelName = utils.NewErrorValue[string](err)
		info.Hostname = utils.NewErrorValue[string](err)
		info.KernelVersion = utils.NewErrorValue[string](unameErr)
	}
}
