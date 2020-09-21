// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package ebpf

import (
	"bytes"
	"errors"
	"io/ioutil"
	"regexp"

	"golang.org/x/sys/unix"
)

const defaultSymFile = "/proc/kallsyms"

// RuntimeArch holds the CPU architecture of the running machine
var RuntimeArch string

// GetSyscallFnName returns the qualified syscall named by going through '/proc/kallsyms' on the
// system on which its executed. It allows BPF programs that may have been compiled
// for older syscall functions to run on newer kernels
func GetSyscallFnName(name string) (string, error) {
	// Get kernel symbols
	syms, err := ioutil.ReadFile(defaultSymFile)
	if err != nil {
		return "", err
	}
	return getSyscallFnNameWithKallsyms(name, string(syms))
}

func getSyscallFnNameWithKallsyms(name string, kallsymsContent string) (string, error) {
	// We should search for new syscall function like "__x64__sys_open"
	// Note the start of word boundary. Should return exactly one string
	regexStr := `(\b__` + RuntimeArch + `_[Ss]y[sS]_` + name + `\b)`
	fnRegex := regexp.MustCompile(regexStr)

	match := fnRegex.FindAllString(kallsymsContent, -1)

	// If nothing found, search for old syscall function to be sure
	if len(match) == 0 {
		newRegexStr := `(\b[Ss]y[sS]_` + name + `\b)`
		fnRegex = regexp.MustCompile(newRegexStr)
		newMatch := fnRegex.FindAllString(kallsymsContent, -1)

		// If we get something like 'sys_open' or 'SyS_open', return
		// either (they have same addr) else, just return original string
		if len(newMatch) >= 1 {
			return newMatch[0], nil
		}
		return "", errors.New("could not find a valid syscall name")
	}

	return match[0], nil
}

func init() {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		panic(err)
	}

	switch string(uname.Machine[:bytes.IndexByte(uname.Machine[:], 0)]) {
	case "x86_64":
		RuntimeArch = "x64"
	default:
		RuntimeArch = "ia32"
	}
}
