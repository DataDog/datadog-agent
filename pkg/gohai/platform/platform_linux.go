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

// getOperatingSystem returns the name of the operating system.
//
// The implementation always returns "GNU/Linux" on Linux, similarly to what `uname -o` does.
func getOperatingSystem() string {
	// eventually we might want to return different values depending on the actual OS
	// (not all Linux are GNU)
	return "GNU/Linux"
}

// isVendorAMD checks if the vendor is AMD.
// The reader is expected be an io.Reader over /proc/cpuinfo
func isVendorAMD(reader io.Reader) bool {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		text := scanner.Text()
		key, value, found := strings.Cut(text, ":")
		if !found {
			continue
		}

		if strings.TrimSpace(key) == "vendor_id" {
			return strings.TrimSpace(value) == "AuthenticAMD"
		}
	}
	return false
}

// getProcessorType returns the processor type, eg. 'x86_64', 'amd64', 'arm', 'i386'.
//
// The implementation is similar to `uname -p`, it uses the machine value but handles specifically
// the edge case of athlon processors on i686 devices
func getProcessorType(machine string) string {
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

// getHardwarePlatform returns the hardware platform, eg. 'i86pc', 'x86_64', 'aarch64'.
//
// The implementation is similar to `uname -i`, it uses the machine value but returns 'i386' for
// all 'i*86' devices
func getHardwarePlatform(machine string) string {
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
	archInfo["processor"] = getProcessorType(machine)
	archInfo["hardware_platform"] = getHardwarePlatform(machine)
	archInfo["os"] = getOperatingSystem()
	archInfo["kernel_version"] = utils.StringFromBytes(uname.Version[:])
}
