// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"bufio"
	"io"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
	"golang.org/x/sys/unix"
)

// getUnameOS is similar to `uname -o`
//
// uname uses preprocessor macros to determine the printable name of the operating system,
// cf https://github.com/coreutils/gnulib/blob/master/m4/host-os.m4
//
// on every Linux device it prints "GNU/Linux", probably because uname is from GNU coreutils,
// so if you are using it your OS is considered to be GNU
func getUnameOS() string {
	// eventually we might want to return different values depending on the actual OS
	// (not all Linux are GNU)
	return "GNU/Linux"
}

// isVendorAMD checks if the vendor is AMD.
// The reader is expecetd be an io.Reader over /proc/cpuinfo
func isVendorAMD(reader io.Reader) bool {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "vendor_id") {
			if strings.Contains(text, "AuthenticAMD") {
				return true
			}
			break
		}
	}
	return false
}

// getUnameProcessor is similar to `uname -p`
//
// the version of uname commonly used on Linux handles specifically the edge case of athlon processors
// on i686 devices
func getUnameProcessor(machine string) string {
	if machine == "i686" {
		file, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer file.Close()
			if isVendorAMD(file) {
				return "athlon"
			}
		}
	}

	return machine
}

// getUnameHardwarePlatform is similar to `uname -i`
//
// the version of uname commonly used on Linux returns 'i386' for all 'i*86' devices
func getUnameHardwarePlatform(machine string) string {
	if len(machine) == 4 && machine[0] == 'i' && machine[2] == '8' && machine[3] == '6' {
		return "i386"
	}
	return machine
}

func updateArchInfo(archInfo map[string]string, uname *unix.Utsname) {
	archInfo["kernel_name"] = utils.StringFromBytes(uname.Sysname[:])
	archInfo["hostname"] = utils.StringFromBytes(uname.Nodename[:])
	archInfo["kernel_release"] = utils.StringFromBytes(uname.Release[:])
	machine := utils.StringFromBytes(uname.Machine[:])
	archInfo["machine"] = machine
	archInfo["processor"] = getUnameProcessor(machine)
	archInfo["hardware_platform"] = getUnameHardwarePlatform(machine)
	archInfo["os"] = getUnameOS()
	archInfo["kernel_version"] = utils.StringFromBytes(uname.Version[:])
}
