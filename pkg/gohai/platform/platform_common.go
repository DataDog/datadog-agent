// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package platform regroups collecting information about the platform
package platform

// Platform holds metadata about the host
type Platform struct {
	// GoVersion is the golang version.
	GoVersion string
	// GoOS is equal to "runtime.GOOS"
	GoOS string
	// GoArch is equal to "runtime.GOARCH"
	GoArch string

	// KernelName is the kernel name (ex:  "windows", "Linux", ...)
	KernelName string
	// KernelRelease the kernel release (ex: "10.0.20348", "4.15.0-1080-gcp", ...)
	KernelRelease string
	// Hostname is the hostname for the host
	Hostname string
	// Machine the architecture for the host (is: x86_64 vs arm).
	Machine string
	// OS is the os name description (ex: "GNU/Linux", "Windows Server 2022 Datacenter", ...)
	OS string

	// Family is the OS family (Windows only)
	Family string

	// KernelVersion the kernel version, Unix only
	KernelVersion string
	// Processor is the processor type, Unix only (ex "x86_64", "arm", ...)
	Processor string
	// HardwarePlatform is the hardware name, Linux only (ex "x86_64")
	HardwarePlatform string
}

const name = "platform"

// Name returns the name of the package
func (platform *Platform) Name() string {
	return name
}
