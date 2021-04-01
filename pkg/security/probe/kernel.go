// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"strings"

	"github.com/cobaugh/osrelease"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)
	kernel4_13 = kernel.VersionCode(4, 13, 0) //nolint:deadcode,unused
	kernel4_16 = kernel.VersionCode(4, 16, 0) //nolint:deadcode,unused
	kernel5_3  = kernel.VersionCode(5, 3, 0)  //nolint:deadcode,unused
)

// KernelVersion defines a kernel version helper
type KernelVersion struct {
	osrelease map[string]string
}

// NewKernelVersion returns a new kernel version helper
func NewKernelVersion() (*KernelVersion, error) {
	osrelease, err := osrelease.Read()
	if err != nil {
		return nil, err
	}
	return &KernelVersion{
		osrelease: osrelease,
	}, nil
}

// IsRH7Kernel returns whether the kernel is a rh7 kernel
func (k *KernelVersion) IsRH7Kernel() bool {
	return (k.osrelease["ID"] == "centos" || k.osrelease["ID"] == "rhel") && k.osrelease["VERSION_ID"] == "7"
}

// IsRH8Kernel returns whether the kernel is a rh8 kernel
func (k *KernelVersion) IsRH8Kernel() bool {
	return k.osrelease["PLATFORM_ID"] == "platform:el8"
}

// IsSuseKernel returns whether the kernel is a suse kernel
func (k *KernelVersion) IsSuseKernel() bool {
	return k.osrelease["ID"] == "sles" || k.osrelease["ID"] == "opensuse-leap"
}

// IsSLES12Kernel returns whether the kernel is a sles 12 kernel
func (k *KernelVersion) IsSLES12Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.osrelease["VERSION_ID"], "12")
}

// IsSLES15Kernel returns whether the kernel is a sles 15 kernel
func (k *KernelVersion) IsSLES15Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.osrelease["VERSION_ID"], "15")
}
