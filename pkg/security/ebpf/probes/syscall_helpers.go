// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

import (
	"bytes"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
)

// RuntimeArch holds the CPU architecture of the running machine
var RuntimeArch string

func resolveRuntimeArch() {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		panic(err)
	}

	switch string(uname.Machine[:bytes.IndexByte(uname.Machine[:], 0)]) {
	case "x86_64":
		RuntimeArch = "x64"
	case "aarch64":
		RuntimeArch = "arm64"
	default:
		RuntimeArch = "ia32"
	}
}

// cache of the syscall prefix depending on kernel version
var syscallPrefix string
var ia32SyscallPrefix string

func getSyscallPrefix() string {
	if syscallPrefix == "" {
		syscall, err := manager.GetSyscallFnName("open")
		if err != nil {
			log.Error(err)
			return "__unknown__"
		}
		syscallPrefix = strings.ToLower(strings.TrimSuffix(syscall, "open"))
		if syscallPrefix != "sys_" {
			ia32SyscallPrefix = "__ia32_"
		} else {
			ia32SyscallPrefix = "compat_"
		}
	}

	return syscallPrefix
}

func getSyscallFnName(name string) string {
	return getSyscallPrefix() + name
}

func getIA32SyscallFnName(name string) string {
	return ia32SyscallPrefix + "sys_" + name
}

func getCompatSyscallFnName(name string) string {
	return ia32SyscallPrefix + "compat_sys_" + name
}

func expandKprobe(hookpoint string, flag int) []string {
	var sections []string
	if flag&Entry == Entry {
		sections = append(sections, "kprobe/"+hookpoint)
	}
	if flag&Exit == Exit {
		sections = append(sections, "kretprobe/"+hookpoint)
	}
	return sections
}

func expandSyscallSections(syscallName string, flag int, compat ...bool) []string {
	sections := expandKprobe(getSyscallFnName(syscallName), flag)

	if RuntimeArch == "x64" {
		if len(compat) > 0 && compat[0] && syscallPrefix != "sys_" {
			sections = append(sections, expandKprobe(getCompatSyscallFnName(syscallName), flag)...)
		} else {
			sections = append(sections, expandKprobe(getIA32SyscallFnName(syscallName), flag)...)
		}
	}

	return sections
}

const (
	// Entry indicates that the entry kprobe should be expanded
	Entry = 1 << 0
	// Exit indicates that the exit kretprobe should be expanded
	Exit = 1 << 1
	// ExpandTime32 indicates that the _time32 suffix should be added to the provided probe if needed
	ExpandTime32 = 1 << 2

	// EntryAndExit indicates that both the entry kprobe and exit kretprobe should be expanded
	EntryAndExit = Entry | Exit
)

// ExpandSyscallProbes returns the list of available hook probes for the syscall func name of the provided probe
func ExpandSyscallProbes(probe *manager.Probe, flag int, compat ...bool) []*manager.Probe {
	var probes []*manager.Probe
	syscallName := probe.SyscallFuncName
	probe.SyscallFuncName = ""

	if len(RuntimeArch) == 0 {
		resolveRuntimeArch()
	}

	if flag&ExpandTime32 == ExpandTime32 {
		// check if _time32 should be expanded
		if getSyscallPrefix() == "sys_" {
			return probes
		}
		syscallName += "_time32"
	}

	for _, section := range expandSyscallSections(syscallName, flag, compat...) {
		probeCopy := probe.Copy()
		probeCopy.Section = section
		probes = append(probes, probeCopy)
	}

	return probes
}

// ExpandSyscallProbesSelector returns the list of a ProbesSelector required to query all the probes available for a syscall
func ExpandSyscallProbesSelector(id manager.ProbeIdentificationPair, flag int, compat ...bool) []manager.ProbesSelector {
	var selectors []manager.ProbesSelector

	if len(RuntimeArch) == 0 {
		resolveRuntimeArch()
	}

	if flag&ExpandTime32 == ExpandTime32 {
		// check if _time32 should be expanded
		if getSyscallPrefix() == "sys_" {
			return selectors
		}
		id.Section += "_time32"
	}

	for _, section := range expandSyscallSections(id.Section, flag, compat...) {
		selector := &manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{UID: id.UID, Section: section}}
		selectors = append(selectors, selector)
	}

	return selectors
}
