// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kernel holds kernel related files
package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/acobaugh/osrelease"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var (
	// KERNEL_VERSION(a,b,c) = (a << 16) + (b << 8) + (c)

	// Kernel4_9 is the KernelVersion representation of kernel version 4.9
	Kernel4_9 = kernel.VersionCode(4, 9, 0)
	// Kernel4_10 is the KernelVersion representation of kernel version 4.10
	Kernel4_10 = kernel.VersionCode(4, 10, 0)
	// Kernel4_12 is the KernelVersion representation of kernel version 4.12
	Kernel4_12 = kernel.VersionCode(4, 12, 0)
	// Kernel4_13 is the KernelVersion representation of kernel version 4.13
	Kernel4_13 = kernel.VersionCode(4, 13, 0)
	// Kernel4_14 is the KernelVersion representation of kernel version 4.14
	Kernel4_14 = kernel.VersionCode(4, 14, 0)
	// Kernel4_15 is the KernelVersion representation of kernel version 4.15
	Kernel4_15 = kernel.VersionCode(4, 15, 0)
	// Kernel4_16 is the KernelVersion representation of kernel version 4.16
	Kernel4_16 = kernel.VersionCode(4, 16, 0)
	// Kernel4_18 is the KernelVersion representation of kernel version 4.18
	Kernel4_18 = kernel.VersionCode(4, 18, 0)
	// Kernel4_19 is the KernelVersion representation of kernel version 4.19
	Kernel4_19 = kernel.VersionCode(4, 19, 0)
	// Kernel4_20 is the KernelVersion representation of kernel version 4.20
	Kernel4_20 = kernel.VersionCode(4, 20, 0)
	// Kernel5_0 is the KernelVersion representation of kernel version 5.0
	Kernel5_0 = kernel.VersionCode(5, 0, 0)
	// Kernel5_1 is the KernelVersion representation of kernel version 5.1
	Kernel5_1 = kernel.VersionCode(5, 1, 0)
	// Kernel5_2 is the KernelVersion representation of kernel version 5.2
	Kernel5_2 = kernel.VersionCode(5, 2, 0)
	// Kernel5_3 is the KernelVersion representation of kernel version 5.3
	Kernel5_3 = kernel.VersionCode(5, 3, 0)
	// Kernel5_4 is the KernelVersion representation of kernel version 5.4
	Kernel5_4 = kernel.VersionCode(5, 4, 0)
	// Kernel5_5 is the KernelVersion representation of kernel version 5.5
	Kernel5_5 = kernel.VersionCode(5, 5, 0)
	// Kernel5_6 is the KernelVersion representation of kernel version 5.6
	Kernel5_6 = kernel.VersionCode(5, 6, 0)
	// Kernel5_7 is the KernelVersion representation of kernel version 5.7
	Kernel5_7 = kernel.VersionCode(5, 7, 0)
	// Kernel5_8 is the KernelVersion representation of kernel version 5.8
	Kernel5_8 = kernel.VersionCode(5, 8, 0)
	// Kernel5_9 is the KernelVersion representation of kernel version 5.9
	Kernel5_9 = kernel.VersionCode(5, 9, 0)
	// Kernel5_10 is the KernelVersion representation of kernel version 5.10
	Kernel5_10 = kernel.VersionCode(5, 10, 0)
	// Kernel5_11 is the KernelVersion representation of kernel version 5.11
	Kernel5_11 = kernel.VersionCode(5, 11, 0)
	// Kernel5_12 is the KernelVersion representation of kernel version 5.12
	Kernel5_12 = kernel.VersionCode(5, 12, 0)
	// Kernel5_13 is the KernelVersion representation of kernel version 5.13
	Kernel5_13 = kernel.VersionCode(5, 13, 0)
	// Kernel5_14 is the KernelVersion representation of kernel version 5.14
	Kernel5_14 = kernel.VersionCode(5, 14, 0)
	// Kernel5_15 is the KernelVersion representation of kernel version 5.15
	Kernel5_15 = kernel.VersionCode(5, 15, 0)
	// Kernel5_16 is the KernelVersion representation of kernel version 5.16
	Kernel5_16 = kernel.VersionCode(5, 16, 0)
	// Kernel5_17 is the KernelVersion representation of kernel version 5.17
	Kernel5_17 = kernel.VersionCode(5, 17, 0)
	// Kernel5_18 is the KernelVersion representation of kernel version 5.18
	Kernel5_18 = kernel.VersionCode(5, 18, 0)
	// Kernel5_19 is the KernelVersion representation of kernel version 5.19
	Kernel5_19 = kernel.VersionCode(5, 19, 0)
	// Kernel6_0 is the KernelVersion representation of kernel version 6.0
	Kernel6_0 = kernel.VersionCode(6, 0, 0)
	// Kernel6_1 is the KernelVersion representation of kernel version 6.1
	Kernel6_1 = kernel.VersionCode(6, 1, 0)
	// Kernel6_2 is the KernelVersion representation of kernel version 6.2
	Kernel6_2 = kernel.VersionCode(6, 2, 0)
	// Kernel6_3 is the KernelVersion representation of kernel version 6.3
	Kernel6_3 = kernel.VersionCode(6, 3, 0)
	// Kernel6_5 is the KernelVersion representation of kernel version 6.5
	Kernel6_5 = kernel.VersionCode(6, 5, 0)
	// Kernel6_6 is the KernelVersion representation of kernel version 6.6
	Kernel6_6 = kernel.VersionCode(6, 6, 0)
	// Kernel6_10 is the KernelVersion representation of kernel version 6.10
	Kernel6_10 = kernel.VersionCode(6, 10, 0)
	// Kernel6_11 is the KernelVersion representation of kernel version 6.11
	Kernel6_11 = kernel.VersionCode(6, 11, 0)
)

// Version defines a kernel version helper
type Version struct {
	OsRelease     map[string]string
	OsReleasePath string
	Code          kernel.Version
	UnameRelease  string
}

func (k *Version) String() string {
	return fmt.Sprintf("kernel %s - %v - %s", k.Code, k.OsRelease, k.UnameRelease)
}

var kernelVersionCache struct {
	sync.RWMutex
	*Version
}

// NewKernelVersion returns a new kernel version helper
func NewKernelVersion() (*Version, error) {
	// fast read path
	kernelVersionCache.RLock()
	if kernelVersionCache.Version != nil {
		kernelVersionCache.RUnlock()
		return kernelVersionCache.Version, nil
	}
	kernelVersionCache.RUnlock()

	// slow write path
	kernelVersionCache.Lock()
	defer kernelVersionCache.Unlock()

	var err error
	kernelVersionCache.Version, err = newKernelVersion()
	return kernelVersionCache.Version, err
}

const lsbRelease = "/etc/lsb-release"

func newKernelVersion() (*Version, error) {
	osReleasePaths := make([]string, 0, 2*3+1)

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
	if env.IsContainerized() && filesystem.FileExists("/host") {
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

	// as a final fallback, we try to read /etc/lsb-release, useful for very old systems
	osReleasePaths = append(osReleasePaths, lsbRelease)

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to detect kernel version: %w", err)
	}

	unameRelease, err := kernel.Release()
	if err != nil {
		return nil, err
	}

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

	return nil, fmt.Errorf("failed to detect operating system version for %s", unameRelease)
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
	return (k.OsRelease["ID"] == "centos" || k.OsRelease["ID"] == "rhel") && strings.HasPrefix(k.OsRelease["VERSION_ID"], "7")
}

// IsRH8Kernel returns whether the kernel is a rh8 kernel
func (k *Version) IsRH8Kernel() bool {
	return k.OsRelease["PLATFORM_ID"] == "platform:el8"
}

// IsRH9Kernel returns whether the kernel is a rh9 kernel
func (k *Version) IsRH9Kernel() bool {
	return k.OsRelease["PLATFORM_ID"] == "platform:el9"
}

// IsRH9_3Kernel returns whether the kernel is a rh9.3 kernel
func (k *Version) IsRH9_3Kernel() bool {
	return k.IsRH9Kernel() && strings.HasPrefix(k.OsRelease["VERSION_ID"], "9.3")
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

// IsOpenSUSELeapKernel returns whether the kernel is an opensuse kernel
func (k *Version) IsOpenSUSELeapKernel() bool {
	return k.OsRelease["ID"] == "opensuse-leap"
}

// IsOpenSUSELeap15_3Kernel returns whether the kernel is an opensuse 15.3 kernel
func (k *Version) IsOpenSUSELeap15_3Kernel() bool {
	return k.IsOpenSUSELeapKernel() && strings.HasPrefix(k.OsRelease["VERSION_ID"], "15.3")
}

// IsOracleUEKKernel returns whether the kernel is an oracle uek kernel
func (k *Version) IsOracleUEKKernel() bool {
	return k.OsRelease["ID"] == "ol" && k.Code >= Kernel5_4
}

// IsCOSKernel returns whether the kernel is a suse kernel
func (k *Version) IsCOSKernel() bool {
	return k.OsRelease["ID"] == "cos"
}

// IsAmazonLinuxKernel returns whether the kernel is an amazon linux kernel
func (k *Version) IsAmazonLinuxKernel() bool {
	return k.OsRelease["ID"] == "amzn"
}

// IsAmazonLinux2022Kernel returns whether the kernel is an amazon linux 2022 kernel
func (k *Version) IsAmazonLinux2022Kernel() bool {
	return k.IsAmazonLinuxKernel() && k.OsRelease["VERSION_ID"] == "2022"
}

// IsAmazonLinux2023Kernel returns whether the kernel is an amazon linux 2023 kernel
func (k *Version) IsAmazonLinux2023Kernel() bool {
	return k.IsAmazonLinuxKernel() && k.OsRelease["VERSION_ID"] == "2023"
}

// IsInRangeCloseOpen returns whether the kernel version is between the begin
// version (included) and the end version (excluded)
func (k *Version) IsInRangeCloseOpen(begin kernel.Version, end kernel.Version) bool {
	return k.Code != 0 && begin <= k.Code && k.Code < end
}

// HaveMmapableMaps returns whether the kernel supports mmapable maps.
func (k *Version) HaveMmapableMaps() bool {
	return features.HaveMapFlag(features.BPF_F_MMAPABLE) == nil

}

// HaveRingBuffers returns whether the kernel supports ring buffer.
func (k *Version) HaveRingBuffers() bool {
	return features.HaveMapType(ebpf.RingBuf) == nil
}

// HavePIDLinkStruct returns whether the kernel uses the pid_link struct, which was removed in 4.19
func (k *Version) HavePIDLinkStruct() bool {
	return k.Code != 0 && k.Code < Kernel4_19 && !k.IsRH8Kernel()
}

// HaveLegacyPipeInodeInfoStruct returns whether the kernel uses the legacy pipe_inode_info struct
func (k *Version) HaveLegacyPipeInodeInfoStruct() bool {
	return k.Code != 0 && k.Code < Kernel5_5
}

// HaveFentrySupport returns whether the kernel supports fentry probes
func (k *Version) HaveFentrySupport() bool {
	if features.HaveProgramType(ebpf.Tracing) != nil {
		return false
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceFEntry,
		AttachTo:   "vfs_open",
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
	}
	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return false
	}
	defer prog.Close()

	link, err := link.AttachTracing(link.TracingOptions{
		Program: prog,
	})
	if err != nil {
		return false
	}
	defer link.Close()

	return true
}

// SupportBPFSendSignal returns true if the eBPF function bpf_send_signal is available
func (k *Version) SupportBPFSendSignal() bool {
	return k.Code != 0 && k.Code >= Kernel5_3
}

// SupportCORE returns is CORE is supported
func (k *Version) SupportCORE() bool {
	_, err := btf.LoadKernelSpec()
	return err == nil
}
