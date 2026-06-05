// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	gopsutilhost "github.com/shirou/gopsutil/v4/host"
	"golang.org/x/sys/unix"
)

// ParseOsLevelToKernelVersion parses the output of `oslevel -s` into
// version, release, TL, and SP components following the pattern
// "version.release-TL-SP-date".
func ParseOsLevelToKernelVersion(osLevel string) string {
	// check that osLevel is in the format "version.release-TL-SP-date"
	parts := strings.SplitN(osLevel, "-", 4)
	if len(parts) < 4 || len(parts[0]) < 2 {
		return ""
	}

	// extract each part of the kernel version
	version := int(parts[0][0]) - '0'
	release := int(parts[0][1]) - '0'
	tl, _ := strconv.Atoi(parts[1])
	sp, _ := strconv.Atoi(parts[2])

	return fmt.Sprintf("%d.%d.%d.%d", version, release, tl, sp)
}

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
	info.Machine = utils.NewValue(runtime.GOARCH)

	// gopsutil provides the full AIX maintenance level via `oslevel -s`
	// (e.g. "7300-02-02-2419") and hostname. We format KernelVersion as
	// "V.R.TL.SP" (e.g. "7.3.2.2") to match VRMF dot notation used by
	// installp/lslpp, which is more useful than the raw dash-separated string.
	hostInfo, err := gopsutilhost.Info()
	if err == nil {
		info.KernelName = utils.NewValue("AIX")
		info.Hostname = utils.NewValue(hostInfo.Hostname)
		info.KernelVersion = utils.NewValue(ParseOsLevelToKernelVersion(hostInfo.KernelVersion))
	} else if unameErr == nil {
		// Fall back to uname fields if gopsutil fails.
		info.KernelName = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
		info.Hostname = utils.NewValue(utils.StringFromBytes(uname.Nodename[:]))

		// Extract first 2 parts of the kernel version ("7.3.1.4" -> "7.3")
		kernelVersion := utils.StringFromBytes(uname.Version[:]) + "." + utils.StringFromBytes(uname.Release[:])
		info.KernelVersion = utils.NewValue(kernelVersion)
	} else {
		// Both gopsutil and uname failed; report the actual errors.
		info.KernelName = utils.NewErrorValue[string](err)
		info.Hostname = utils.NewErrorValue[string](err)
		info.KernelVersion = utils.NewErrorValue[string](unameErr)
	}
}
