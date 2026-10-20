// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"

	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
)

// gpuDeviceTagRegex is the gpu_device value format of pkg/collector/corechecks/gpu/spec/tags.yaml.
var gpuDeviceTagRegex = regexp.MustCompile(`^[a-z0-9_.-]+$`)

func TestInstinctNamesAreValidTags(t *testing.T) {
	for id, name := range instinctNames {
		tag := gpuutil.NormalizeGPUDeviceName(cleanName(name))
		assert.Regexp(t, gpuDeviceTagRegex, tag, "device 0x%04x %q", id, name)
		assert.NotEmpty(t, gpuutil.ExtractGPUType(cleanName(name)), "device 0x%04x %q has no gpu_type", id, name)
	}
}

func TestCleanName(t *testing.T) {
	assert.Equal(t, "AMD Instinct MI250X MI250", cleanName("AMD Instinct MI250X / MI250"))
	assert.Equal(t, "AMD Instinct MI350X VF", cleanName("AMD Instinct MI350X VF"))
	assert.Equal(t, "AMD Radeon TM Pro W7900", cleanName("AMD Radeon (TM) Pro  W7900"))
}
