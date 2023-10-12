// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

//go:build !android

// Package platform regroups collecting information about the platform
package platform

import (
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// Info holds metadata about the host
type Info struct {
	// GoVersion is the golang version.
	GoVersion utils.Value[string] `json:"goV"`
	// GoOS is equal to "runtime.GOOS"
	GoOS utils.Value[string] `json:"GOOS"`
	// GoArch is equal to "runtime.GOARCH"
	GoArch utils.Value[string] `json:"GOOARCH"`

	// KernelName is the kernel name (ex:  "windows", "Linux", ...)
	KernelName utils.Value[string] `json:"kernel_name"`
	// KernelRelease the kernel release (ex: "10.0.20348", "4.15.0-1080-gcp", ...)
	KernelRelease utils.Value[string] `json:"kernel_release"`
	// Hostname is the hostname for the host
	Hostname utils.Value[string] `json:"hostname"`
	// Machine the architecture for the host (is: x86_64 vs arm).
	Machine utils.Value[string] `json:"machine"`
	// OS is the os name description (ex: "GNU/Linux", "Windows Server 2022 Datacenter", ...)
	OS utils.Value[string] `json:"os"`

	// Family is the OS family (Windows only)
	Family utils.Value[string] `json:"family"`

	// KernelVersion the kernel version, Unix only
	KernelVersion utils.Value[string] `json:"kernel_version"`
	// Processor is the processor type, Unix only (ex "x86_64", "arm", ...)
	Processor utils.Value[string] `json:"processor"`
	// HardwarePlatform is the hardware name, Linux only (ex "x86_64")
	HardwarePlatform utils.Value[string] `json:"hardware_platform"`
}

// CollectInfo returns an Info struct with every field initialized either to a value or an error.
// The method will try to collect as many fields as possible.
func CollectInfo() *Info {
	info := &Info{
		GoVersion: utils.NewValue(strings.ReplaceAll(runtime.Version(), "go", "")),
		GoOS:      utils.NewValue(runtime.GOOS),
		GoArch:    utils.NewValue(runtime.GOARCH),
	}
	info.fillPlatformInfo()
	return info
}

// AsJSON returns an interface which can be marshalled to a JSON and contains the value of non-errored fields.
func (info *Info) AsJSON() (interface{}, []string, error) {
	return utils.AsJSON(info, false)
}
