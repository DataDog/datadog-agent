// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package flare

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"syscall"
)

func zipLinuxFile(source, tempDir, hostname, filename string) error {
	return zipFile(source, filepath.Join(tempDir, hostname), filename)
}

func zipLinuxKernelSymbols(tempDir, hostname string) error {
	return zipLinuxFile("/proc", tempDir, hostname, "kallsyms")
}

func zipLinuxKprobeEvents(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "kprobe_events")
}

func zipLinuxPid1MountInfo(tempDir, hostname string) error {
	return zipLinuxFile("/proc/1", tempDir, hostname, "mountinfo")
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

func zipLinuxDmesg(tempDir, hostname string) error {
	dmesg, err := readAllDmesg()
	if err != nil {
		return err
	}

	buffer, err := parseDmesg(dmesg)
	if err != nil {
		return err
	}

	return zipReader(bytes.NewBufferString(buffer), filepath.Join(tempDir, hostname), "dmesg")
}

func zipLinuxTracingAvailableEvents(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "available_events")
}

func zipLinuxTracingAvailableFilterFunctions(tempDir, hostname string) error {
	return zipLinuxFile("/sys/kernel/debug/tracing", tempDir, hostname, "available_filter_functions")
}
