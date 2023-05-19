// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// CgroupSysPath returns the path to the provided file within the provided cgroup
func CgroupSysPath(controller string, path string, file string) string {
	return filepath.Join(util.HostSys("fs/cgroup/", controller, path, file))
}

// ReadCgroupFile reads the content of a cgroup file
func ReadCgroupFile(controller string, path string, file string) ([]byte, string, error) {
	p := CgroupSysPath(controller, path, file)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, "", fmt.Errorf("couldn't read %s: %w", p, err)
	}
	return data, p, nil
}

// ParseCgroupFileValue parses the content of a cgroup file into an int
func ParseCgroupFileValue(controller string, path string, file string) (int, error) {
	data, cgroupPath, err := ReadCgroupFile(controller, path, file)
	if err != nil {
		return 0, err
	}

	value, err := strconv.Atoi(strings.Trim(string(data), "\n"))
	if err != nil {
		return 0, fmt.Errorf("couldn't parse %s: %w", cgroupPath, err)
	}

	return value, nil
}
