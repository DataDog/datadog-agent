// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package flare

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/DataDog/ebpf-manager/tracefs"

	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/config"
)

func addSystemProbePlatformSpecificEntries(fb flaretypes.FlareBuilder) {
	sysprobeSocketLocation := config.SystemProbe.GetString("system_probe_config.sysprobe_socket")
	if sysprobeSocketLocation != "" {
		fb.RegisterDirPerm(filepath.Dir(sysprobeSocketLocation))
	}
}

func getLinuxKernelSymbols(fb flaretypes.FlareBuilder) error {
	return fb.CopyFile("/proc/kallsyms")
}

func getLinuxKprobeEvents(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "kprobe_events"))
}

func getLinuxPid1MountInfo(fb flaretypes.FlareBuilder) error {
	return fb.CopyFile("/proc/1/mountinfo")
}

var klogRegexp = regexp.MustCompile(`<(\d+)>(.*)`)

func readAllDmesg() ([]byte, error) {
	const syslogActionSizeBuffer = 10
	const syslogActionReadAll = 3

	n, err := syscall.Klogctl(syslogActionSizeBuffer, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to query size of log buffer [%w]", err)
	}

	b := make([]byte, n)

	m, err := syscall.Klogctl(syslogActionReadAll, b)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages from log buffer [%w]", err)
	}

	return b[:m], nil
}

func parseDmesg(buffer []byte) (string, error) {
	buf := bytes.NewBuffer(buffer)
	var result string

	for {
		line, err := buf.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return result, err
		}

		parts := klogRegexp.FindStringSubmatch(line)
		if parts != nil {
			result += parts[2] + "\n"
		} else {
			result += line
		}
	}

	return result, nil
}

func getLinuxDmesg(fb flaretypes.FlareBuilder) error {
	dmesg, err := readAllDmesg()
	if err != nil {
		return err
	}

	content, err := parseDmesg(dmesg)
	if err != nil {
		return err
	}

	return fb.AddFile("dmesg", []byte(content))
}

func getLinuxTracingAvailableEvents(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "available_events"))
}

func getLinuxTracingAvailableFilterFunctions(fb flaretypes.FlareBuilder) error {
	traceFSPath, err := tracefs.Root()
	if err != nil {
		return err
	}
	return fb.CopyFile(filepath.Join(traceFSPath, "available_filter_functions"))
}
