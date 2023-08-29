// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"errors"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	log "github.com/cihub/seelog"
	"golang.org/x/sys/unix"
)

// getUnameProcessor is similar to `uname -p`
//
// for Apple devices, uname does as follow to determine the value:
// - if the architecture is arm or arm64, return "arm"
// - if the architecture is i386 or x86_64, return "i386"
// - if the architecture is powerpc or powerpc64, return "powerpc"
// - return "unknown"
//
// cf https://github.com/coreutils/coreutils/blob/master/src/uname.c
func getUnameProcessor() string {
	platforms := map[string]string{
		"arm":     "arm",
		"arm64":   "arm",
		"amd64":   "i386",
		"386":     "i386",
		"ppc64":   "powerpc",
		"ppc64le": "powerpc",
	}

	processor, ok := platforms[runtime.GOARCH]
	if ok {
		return processor
	}

	return "unknown"
}

// processIsTranslated detects if the process using gohai is running under the Rosetta 2 translator
func processIsTranslated() (bool, error) {
	// https://developer.apple.com/documentation/apple_silicon/about_the_rosetta_translation_environment#3616845
	ret, err := unix.SysctlUint32("sysctl.proc_translated")

	if err == nil {
		return ret == 1, nil
	}

	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	return false, err
}

func updateUnameInfo(platformInfo *Info, uname *unix.Utsname) {
	platformInfo.KernelName = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
	platformInfo.Hostname = utils.NewValue(utils.StringFromBytes(uname.Nodename[:]))
	platformInfo.KernelRelease = utils.NewValue(utils.StringFromBytes(uname.Release[:]))
	platformInfo.Machine = utils.NewValue(utils.StringFromBytes(uname.Machine[:]))
	// for backward-compatibility reasons we just use the Sysname field
	platformInfo.OS = utils.NewValue(utils.StringFromBytes(uname.Sysname[:]))
	platformInfo.KernelVersion = utils.NewValue(utils.StringFromBytes(uname.Version[:]))
}

func (platformInfo *Info) fillPlatformInfo() {
	platformInfo.HardwarePlatform = utils.NewErrorValue[string](utils.ErrNotCollectable)
	platformInfo.Family = utils.NewErrorValue[string](utils.ErrNotCollectable)

	platformInfo.Processor = utils.NewValue(getUnameProcessor())

	var uname unix.Utsname
	unameErr := unix.Uname(&uname)
	if unameErr == nil {
		updateUnameInfo(platformInfo, &uname)
	} else {
		failedFields := []*utils.Value[string]{
			&platformInfo.KernelName, &platformInfo.Hostname, &platformInfo.KernelRelease,
			&platformInfo.Machine, &platformInfo.OS, &platformInfo.KernelVersion,
		}
		for _, field := range failedFields {
			(*field) = utils.NewErrorValue[string](unameErr)
		}
	}

	if isTranslated, err := processIsTranslated(); err == nil && isTranslated {
		log.Debug("Running under Rosetta translator; overriding architecture values")
		platformInfo.Processor = utils.NewValue("arm")
		platformInfo.Machine = utils.NewValue("arm64")
	} else if err != nil {
		log.Debugf("Error when detecting Rosetta translator: %s", err)
	}
}
