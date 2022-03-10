// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/acobaugh/osrelease"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)

	// Kernel4_12 is the KernelVersion representation of kernel version 4.12
	Kernel4_12 = kernel.VersionCode(4, 12, 0) //nolint:deadcode,unused
	// Kernel4_13 is the KernelVersion representation of kernel version 4.13
	Kernel4_13 = kernel.VersionCode(4, 13, 0) //nolint:deadcode,unused
	// Kernel4_14 is the KernelVersion representation of kernel version 4.14
	Kernel4_14 = kernel.VersionCode(4, 14, 0) //nolint:deadcode,unused
	// Kernel4_15 is the KernelVersion representation of kernel version 4.15
	Kernel4_15 = kernel.VersionCode(4, 15, 0) //nolint:deadcode,unused
	// Kernel4_16 is the KernelVersion representation of kernel version 4.16
	Kernel4_16 = kernel.VersionCode(4, 16, 0) //nolint:deadcode,unused
	// Kernel4_18 is the KernelVersion representation of kernel version 4.18
	Kernel4_18 = kernel.VersionCode(4, 18, 0) //nolint:deadcode,unused
	// Kernel4_19 is the KernelVersion representation of kernel version 4.19
	Kernel4_19 = kernel.VersionCode(4, 19, 0) //nolint:deadcode,unused
	// Kernel4_20 is the KernelVersion representation of kernel version 4.20
	Kernel4_20 = kernel.VersionCode(4, 20, 0) //nolint:deadcode,unused
	// Kernel5_0 is the KernelVersion representation of kernel version 5.0
	Kernel5_0 = kernel.VersionCode(5, 0, 0) //nolint:deadcode,unused
	// Kernel5_1 is the KernelVersion representation of kernel version 5.1
	Kernel5_1 = kernel.VersionCode(5, 1, 0) //nolint:deadcode,unused
	// Kernel5_3 is the KernelVersion representation of kernel version 5.3
	Kernel5_3 = kernel.VersionCode(5, 3, 0) //nolint:deadcode,unused
	// Kernel5_4 is the KernelVersion representation of kernel version 5.4
	Kernel5_4 = kernel.VersionCode(5, 4, 0) //nolint:deadcode,unused
	// Kernel5_5 is the KernelVersion representation of kernel version 5.5
	Kernel5_5 = kernel.VersionCode(5, 5, 0) //nolint:deadcode,unused
	// Kernel5_7 is the KernelVersion representation of kernel version 5.7
	Kernel5_7 = kernel.VersionCode(5, 7, 0) //nolint:deadcode,unused
	// Kernel5_8 is the KernelVersion representation of kernel version 5.8
	Kernel5_8 = kernel.VersionCode(5, 8, 0) //nolint:deadcode,unused
	// Kernel5_9 is the KernelVersion representation of kernel version 5.9
	Kernel5_9 = kernel.VersionCode(5, 9, 0) //nolint:deadcode,unused
	// Kernel5_10 is the KernelVersion representation of kernel version 5.10
	Kernel5_10 = kernel.VersionCode(5, 10, 0) //nolint:deadcode,unused
	// Kernel5_11 is the KernelVersion representation of kernel version 5.11
	Kernel5_11 = kernel.VersionCode(5, 11, 0) //nolint:deadcode,unused
	// Kernel5_12 is the KernelVersion representation of kernel version 5.12
	Kernel5_12 = kernel.VersionCode(5, 12, 0) //nolint:deadcode,unused
	// Kernel5_13 is the KernelVersion representation of kernel version 5.13
	Kernel5_13 = kernel.VersionCode(5, 13, 0) //nolint:deadcode,unused
	// Kernel5_14 is the KernelVersion representation of kernel version 5.14
	Kernel5_14 = kernel.VersionCode(5, 14, 0) //nolint:deadcode,unused
	// Kernel5_16 is the KernelVersion representation of kernel version 5.16
	Kernel5_16 = kernel.VersionCode(5, 16, 0) //nolint:deadcode,unused
)

// Version defines a kernel version helper
type Version struct {
	osRelease map[string]string
	Code      kernel.Version
}

func (k *Version) String() string {
	return fmt.Sprintf("kernel %s - %v", k.Code, k.osRelease)
}

// NewKernelVersion returns a new kernel version helper
func NewKernelVersion() (*Version, error) {
	osReleasePaths := []string{
		osrelease.EtcOsRelease,
		osrelease.UsrLibOsRelease,
	}

	if config.IsContainerized() && util.PathExists("/host") {
		osReleasePaths = append([]string{
			filepath.Join("/host", osrelease.EtcOsRelease),
			filepath.Join("/host", osrelease.UsrLibOsRelease),
		}, osReleasePaths...)
	}

	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		osReleasePaths = append([]string{
			filepath.Join(hostRoot, osrelease.EtcOsRelease),
			filepath.Join(hostRoot, osrelease.UsrLibOsRelease),
		}, osReleasePaths...)
	}

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, errors.New("failed to detect kernel version")
	}

	var release map[string]string
	for _, osReleasePath := range osReleasePaths {
		release, err = osrelease.ReadFile(osReleasePath)
		if err == nil {
			return &Version{
				osRelease: release,
				Code:      kv,
			}, nil
		}
	}

	return nil, errors.New("failed to detect operating system version")
}

// IsRH7Kernel returns whether the kernel is a rh7 kernel
func (k *Version) IsRH7Kernel() bool {
	return (k.osRelease["ID"] == "centos" || k.osRelease["ID"] == "rhel") && k.osRelease["VERSION_ID"] == "7"
}

// IsRH8Kernel returns whether the kernel is a rh8 kernel
func (k *Version) IsRH8Kernel() bool {
	return k.osRelease["PLATFORM_ID"] == "platform:el8"
}

// IsSuseKernel returns whether the kernel is a suse kernel
func (k *Version) IsSuseKernel() bool {
	return k.osRelease["ID"] == "sles" || k.osRelease["ID"] == "opensuse-leap"
}

// IsSLES12Kernel returns whether the kernel is a sles 12 kernel
func (k *Version) IsSLES12Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.osRelease["VERSION_ID"], "12")
}

// IsSLES15Kernel returns whether the kernel is a sles 15 kernel
func (k *Version) IsSLES15Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.osRelease["VERSION_ID"], "15")
}

// IsOracleUEKKernel returns whether the kernel is an oracle uek kernel
func (k *Version) IsOracleUEKKernel() bool {
	return k.osRelease["ID"] == "ol" && k.Code >= Kernel5_4
}

// IsCOSKernel returns whether the kernel is a suse kernel
func (k *Version) IsCOSKernel() bool {
	return k.osRelease["ID"] == "cos"
}

// IsAmazonLinuxKernel returns whether the kernel is an amazon kernel
func (k *Version) IsAmazonLinuxKernel() bool {
	return k.osRelease["ID"] == "amzn"
}

// IsInRangeCloseOpen returns whether the kernel version is between the begin
// version (included) and the end version (excluded)
func (k *Version) IsInRangeCloseOpen(begin kernel.Version, end kernel.Version) bool {
	return k.Code != 0 && begin <= k.Code && k.Code < end
}
