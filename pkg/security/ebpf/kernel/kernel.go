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
	"sync"

	"github.com/acobaugh/osrelease"
	"github.com/cilium/ebpf"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/DataDog/btf-internals/sys"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)

	// Kernel4_9 is the KernelVersion representation of kernel version 4.9
	Kernel4_9 = kernel.VersionCode(4, 9, 0) //nolint:deadcode,unused
	// Kernel4_10 is the KernelVersion representation of kernel version 4.10
	Kernel4_10 = kernel.VersionCode(4, 10, 0) //nolint:deadcode,unused
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
	// Kernel5_6 is the KernelVersion representation of kernel version 5.6
	Kernel5_6 = kernel.VersionCode(5, 6, 0) //nolint:deadcode,unused
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
	// Kernel5_15 is the KernelVersion representation of kernel version 5.15
	Kernel5_15 = kernel.VersionCode(5, 15, 0) //nolint:deadcode,unused
	// Kernel5_16 is the KernelVersion representation of kernel version 5.16
	Kernel5_16 = kernel.VersionCode(5, 16, 0) //nolint:deadcode,unused
)

// Version defines a kernel version helper
type Version struct {
	OsRelease     map[string]string
	OsReleasePath string
	Code          kernel.Version
	UnameRelease  string

	haveMmapableMaps *bool
	haveRingBuffers  *bool
}

func (k *Version) String() string {
	return fmt.Sprintf("kernel %s - %v - %s", k.Code, k.OsRelease, k.UnameRelease)
}

var kernelVersionCache struct {
	sync.Mutex
	*Version
}

// NewKernelVersion returns a new kernel version helper
func NewKernelVersion() (*Version, error) {
	kernelVersionCache.Lock()
	defer kernelVersionCache.Unlock()

	if kernelVersionCache.Version != nil {
		return kernelVersionCache.Version, nil
	}

	var err error
	kernelVersionCache.Version, err = newKernelVersion()
	return kernelVersionCache.Version, err
}

func newKernelVersion() (*Version, error) {
	osReleasePaths := make([]string, 0, 2*3)

	// First look at os-release files based on the `HOST_ROOT` env variable
	if hostRoot := os.Getenv("HOST_ROOT"); hostRoot != "" {
		osReleasePaths = append(
			osReleasePaths,
			filepath.Join(hostRoot, osrelease.UsrLibOsRelease),
			filepath.Join(hostRoot, osrelease.EtcOsRelease),
		)
	}

	// Then look if `/host` is mounted in the container
	// since this can be done without the env variable being set
	if config.IsContainerized() && util.PathExists("/host") {
		osReleasePaths = append(
			osReleasePaths,
			filepath.Join("/host", osrelease.UsrLibOsRelease),
			filepath.Join("/host", osrelease.EtcOsRelease),
		)
	}

	// Finally default to actual default values
	// This is last in the search order since we don't want os-release files
	// from the distribution of the container when deployed on a host with
	// different values
	osReleasePaths = append(
		osReleasePaths,
		osrelease.UsrLibOsRelease,
		osrelease.EtcOsRelease,
	)

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to detect kernel version: %w", err)
	}

	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return nil, fmt.Errorf("error calling uname: %w", err)
	}
	unameRelease := unix.ByteSliceToString(uname.Release[:])

	var release map[string]string
	for _, osReleasePath := range osReleasePaths {
		release, err = osrelease.ReadFile(osReleasePath)
		if err == nil {
			return &Version{
				OsRelease:     release,
				OsReleasePath: osReleasePath,
				Code:          kv,
				UnameRelease:  unameRelease,
			}, nil
		}
	}

	return nil, errors.New("failed to detect operating system version")
}

// IsDebianKernel returns whether the kernel is a debian kernel
func (k *Version) IsDebianKernel() bool {
	return k.OsRelease["ID"] == "debian"
}

// IsUbuntuKernel returns whether the kernel is an ubuntu kernel
func (k *Version) IsUbuntuKernel() bool {
	return k.OsRelease["ID"] == "ubuntu"
}

// UbuntuKernelVersion returns a parsed ubuntu kernel version or nil if not on ubuntu or if parsing failed
func (k *Version) UbuntuKernelVersion() *kernel.UbuntuKernelVersion {
	if k.OsRelease["ID"] != "ubuntu" {
		return nil
	}

	ukv, err := kernel.NewUbuntuKernelVersion(k.UnameRelease)
	if err != nil {
		return nil
	}
	return ukv
}

// IsRH7Kernel returns whether the kernel is a rh7 kernel
func (k *Version) IsRH7Kernel() bool {
	return (k.OsRelease["ID"] == "centos" || k.OsRelease["ID"] == "rhel") && k.OsRelease["VERSION_ID"] == "7"
}

// IsRH8Kernel returns whether the kernel is a rh8 kernel
func (k *Version) IsRH8Kernel() bool {
	return k.OsRelease["PLATFORM_ID"] == "platform:el8"
}

// IsSuseKernel returns whether the kernel is a suse kernel
func (k *Version) IsSuseKernel() bool {
	return k.IsSLESKernel() || k.OsRelease["ID"] == "opensuse-leap"
}

// IsSuse12Kernel returns whether the kernel is a sles 12 kernel
func (k *Version) IsSuse12Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.OsRelease["VERSION_ID"], "12")
}

// IsSuse15Kernel returns whether the kernel is a sles 15 kernel
func (k *Version) IsSuse15Kernel() bool {
	return k.IsSuseKernel() && strings.HasPrefix(k.OsRelease["VERSION_ID"], "15")
}

// IsSLESKernel returns whether the kernel is a sles kernel
func (k *Version) IsSLESKernel() bool {
	return k.OsRelease["ID"] == "sles"
}

// IsOracleUEKKernel returns whether the kernel is an oracle uek kernel
func (k *Version) IsOracleUEKKernel() bool {
	return k.OsRelease["ID"] == "ol" && k.Code >= Kernel5_4
}

// IsCOSKernel returns whether the kernel is a suse kernel
func (k *Version) IsCOSKernel() bool {
	return k.OsRelease["ID"] == "cos"
}

// IsAmazonLinuxKernel returns whether the kernel is an amazon kernel
func (k *Version) IsAmazonLinuxKernel() bool {
	return k.OsRelease["ID"] == "amzn"
}

// IsInRangeCloseOpen returns whether the kernel version is between the begin
// version (included) and the end version (excluded)
func (k *Version) IsInRangeCloseOpen(begin kernel.Version, end kernel.Version) bool {
	return k.Code != 0 && begin <= k.Code && k.Code < end
}

func (k *Version) HaveMmapableMaps() bool {
	if k.haveMmapableMaps != nil {
		return *k.haveMmapableMaps
	}

	// This checks BPF_F_MMAPABLE, which appeared in 5.5 for array maps.
	m, err := sys.MapCreate(&sys.MapCreateAttr{
		MapType:    sys.MapType(ebpf.Array),
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
		MapFlags:   unix.BPF_F_MMAPABLE,
	})
	k.haveMmapableMaps = new(bool)
	*k.haveMmapableMaps = err == nil

	if err != nil {
		return false
	}
	_ = m.Close()
	return true
}

func (k *Version) HaveRingBuffers() bool {
	if k.haveRingBuffers != nil {
		return *k.haveRingBuffers
	}

	// This checks ring buffer maps, which appeared in ???.
	m, err := sys.MapCreate(&sys.MapCreateAttr{
		MapType:    sys.MapType(ebpf.RingBuf),
		MaxEntries: 4096 * 16,
	})
	k.haveRingBuffers = new(bool)
	*k.haveRingBuffers = err == nil

	if err != nil {
		return false
	}
	_ = m.Close()
	return true
}
