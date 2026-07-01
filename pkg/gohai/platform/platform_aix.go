// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	gopsutilhost "github.com/shirou/gopsutil/v4/host"
	"golang.org/x/sys/unix"
)

// osLevelRegex matches oslevel -s output (e.g. "7300-02-02-2419") and captures
// version, release, TL, and SP. The 0* prefix on TL and SP strips leading zeros.
var osLevelRegex = regexp.MustCompile(`^(\d)(\d)\d{2}-0*(\d+)-0*(\d+)-\d{4}$`)

// AIXVersion holds the VRMF components parsed from an oslevel -s output string.
type AIXVersion struct {
	Version, Release, TL, SP int
}

func (v *AIXVersion) KernelVersion() string {
	return fmt.Sprintf("%d.%d.%d.%d", v.Version, v.Release, v.TL, v.SP)
}

func (v *AIXVersion) PlatformVersion() string {
	return fmt.Sprintf("%d.%d TL%d", v.Version, v.Release, v.TL)
}

// ParseAIXVersion parses an oslevel -s string (e.g. "7300-02-02-2419") into its
// VRMF components. Returns false if the string is not a valid oslevel -s output.
func ParseAIXVersion(osLevel string) (AIXVersion, bool) {
	matches := osLevelRegex.FindStringSubmatch(osLevel)
	if matches == nil {
		return AIXVersion{}, false
	}
	v, _ := strconv.Atoi(matches[1])
	r, _ := strconv.Atoi(matches[2])
	tl, _ := strconv.Atoi(matches[3])
	sp, _ := strconv.Atoi(matches[4])
	return AIXVersion{Version: v, Release: r, TL: tl, SP: sp}, true
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
		// KernelVersion is formatted as VRMF dot notation (e.g. "7.3.2.2") derived
		// from oslevel -s output (e.g. "7300-02-02-2419"), which is more useful than
		// the raw dash-separated string used by installp/lslpp.
		if aixVersion, ok := ParseAIXVersion(hostInfo.KernelVersion); ok {
			info.KernelVersion = utils.NewValue(aixVersion.KernelVersion())
		} else {
			info.KernelVersion = utils.NewValue(hostInfo.KernelVersion)
		}
	} else if unameErr == nil {
		// Fall back to uname fields if gopsutil fails.
		info.KernelName = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
		info.Hostname = utils.NewValue(utils.StringFromBytes(uname.Nodename[:]))

		// Extract version and release available through uname.
		kernelVersion := fmt.Sprintf("%s.%s", utils.StringFromBytes(uname.Version[:]), utils.StringFromBytes(uname.Release[:]))
		info.KernelVersion = utils.NewValue(kernelVersion)
	} else {
		// Both gopsutil and uname failed; report the actual errors.
		info.KernelName = utils.NewErrorValue[string](err)
		info.Hostname = utils.NewErrorValue[string](err)
		info.KernelVersion = utils.NewErrorValue[string](unameErr)
	}
}
