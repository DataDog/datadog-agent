// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package sysctl is used to process and analyze sysctl data
package sysctl

import (
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestParseCPUFlags(t *testing.T) {
	ours, err := parseCPUFlags()
	if err != nil {
		t.Fatal(err)
	}

	theirs, err := gopsutilVersion()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, ours, theirs)
}

func gopsutilVersion() ([]string, error) {
	info, err := cpu.Info()
	if err != nil {
		return nil, err
	}
	if len(info) == 0 {
		return nil, nil
	}

	return info[0].Flags, nil
}
