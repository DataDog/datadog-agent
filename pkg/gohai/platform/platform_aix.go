// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build aix

package platform

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

func (info *Info) fillPlatformInfo() {
	info.Family = utils.NewErrorValue[string](utils.ErrNotCollectable)
	info.OS = utils.NewValue("AIX")

	var uname unix.Utsname
	if err := unix.Uname(&uname); err == nil {
		info.KernelName = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
		info.Hostname = utils.NewValue(utils.StringFromBytes(uname.Nodename[:]))
		info.KernelRelease = utils.NewValue(utils.StringFromBytes(uname.Release[:]))
		machine := utils.StringFromBytes(uname.Machine[:])
		info.Machine = utils.NewValue(machine)
		info.Processor = utils.NewValue(machine)
		info.HardwarePlatform = utils.NewValue(machine)
		info.KernelVersion = utils.NewValue(utils.StringFromBytes(uname.Version[:]))
	} else {
		info.KernelName = utils.NewErrorValue[string](err)
		info.Hostname = utils.NewErrorValue[string](err)
		info.KernelRelease = utils.NewErrorValue[string](err)
		info.Machine = utils.NewErrorValue[string](err)
		info.Processor = utils.NewErrorValue[string](err)
		info.HardwarePlatform = utils.NewErrorValue[string](err)
		info.KernelVersion = utils.NewErrorValue[string](err)
	}
}
