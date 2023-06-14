// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package platform

import (
	"bufio"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"strings"
)

// getUnameOS returns the same value as `uname -o`
//
// uname uses preprocessor macros to determine the printable name of the operating system,
// cf https://github.com/coreutils/gnulib/blob/master/m4/host-os.m4
//
// on every linux device it prints "GNU/Linux", probably because uname is from GNU coreutils,
// so if you are using it your OS is considered to be GNU
func getUnameOS() string {
	// we might want to return different values depending on the actual OS (not all Linux are GNU)
	// but for now it's good enough
	return "GNU/Linux"
}

// from the 80_fedora_sysinfo.patch uname patch
func procIsAthlon(reader io.Reader) bool {
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

// getUnameProcessor returns the same value as `uname -p`
//
// note that each OS applies a set of patches on the base coreutils source, in particular the 80_fedora_sysinfo.patch
// which makes uname work on Linux, so you have to look at the patched code, eg. for ubuntu jammy:
// https://launchpad.net/ubuntu/+archive/primary/+sourcefiles/coreutils/8.32-4.1ubuntu1/coreutils_8.32-4.1ubuntu1.debian.tar.xz
func getUnameProcessor(machine string) string {
	if machine == "i686" {
		file, err := os.Open("/proc/cpuinfo")
		if err == nil {
			defer file.Close()
			if procIsAthlon(file) {
				machine = "athlon"
			}
		}
	}

	return machine
}

// getUnameProcessor returns the same value as `uname -i`
//
// note that each OS applies a set of patches on the base coreutils source, in particular the 80_fedora_sysinfo.patch
// which makes uname work on Linux, so you have to look at the patched code, eg. for ubuntu jammy:
// https://launchpad.net/ubuntu/+archive/primary/+sourcefiles/coreutils/8.32-4.1ubuntu1/coreutils_8.32-4.1ubuntu1.debian.tar.xz
func getUnameHardwarePlatform(machine string) string {
	if len(machine) == 4 && machine[0] == 'i' && machine[2] == '8' && machine[3] == '6' {
		machine = "i386"
	}
	return machine
}

func updateArchInfo(archInfo map[string]string, uname *unix.Utsname) {
	archInfo["kernel_name"] = string(uname.Sysname[:])
	archInfo["hostname"] = string(uname.Nodename[:])
	archInfo["kernel_release"] = string(uname.Release[:])
	machine := string(uname.Machine[:])
	archInfo["machine"] = machine
	archInfo["processor"] = getUnameProcessor(machine)
	archInfo["hardware_platform"] = getUnameHardwarePlatform(machine)
	archInfo["os"] = getUnameOS()
	archInfo["kernel_version"] = string(uname.Version[:])
}
