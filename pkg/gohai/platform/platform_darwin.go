// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
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
	} else if err.(unix.Errno) == unix.ENOENT {
		return false, nil
	}
	return false, err
}

func updateArchInfo(archInfo map[string]string, uname *unix.Utsname) {
	archInfo["kernel_name"] = utils.StringFromBytes(uname.Sysname[:])
	archInfo["hostname"] = utils.StringFromBytes(uname.Nodename[:])
	archInfo["kernel_release"] = utils.StringFromBytes(uname.Release[:])
	archInfo["machine"] = utils.StringFromBytes(uname.Machine[:])
	archInfo["processor"] = getUnameProcessor()
	// for backward-compatibility reasons we just use the Sysname field
	archInfo["os"] = utils.StringFromBytes(uname.Sysname[:])
	archInfo["kernel_version"] = utils.StringFromBytes(uname.Version[:])

	if isTranslated, err := processIsTranslated(); err == nil && isTranslated {
		log.Debug("Running under Rosetta translator; overriding architecture values")
		archInfo["processor"] = "arm"
		archInfo["machine"] = "arm64"
	} else if err != nil {
		log.Debugf("Error when detecting Rosetta translator: %s", err)
	}
}
