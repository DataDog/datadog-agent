// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sysctl is used to process and analyze sysctl data
package sysctl

import (
	"bufio"
	"os"
	"strings"
)

func parseCPUFlags() ([]string, error) {
	// no need for host proc path here, the cpuinfo file is always exposed
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key != "flags" && key != "Features" {
			continue
		}

		value = strings.TrimSpace(value)

		flags := strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' '
		})
		return flags, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, nil
}
