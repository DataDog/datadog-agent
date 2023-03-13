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

	"github.com/moby/sys/mountinfo"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
)

// MountInfoPidPath returns the path to the mountinfo file of a pid in /proc
func MountInfoPidPath(pid int32) string {
	return filepath.Join(util.HostProc(), fmt.Sprintf("/%d/mountinfo", pid))
}

// ParseMountInfoFile collects the mounts for a specific process ID.
func ParseMountInfoFile(pid int32) ([]*mountinfo.Info, error) {
	f, err := os.Open(MountInfoPidPath(pid))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return mountinfo.GetMountsFromReader(f, nil)
}

// GetMountPoint returns the mount point of the given path
func GetMountPoint(path string) (*mountinfo.Info, error) {
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}

	mi, err := mountinfo.GetMounts(nil)
	if err != nil {
		return nil, err
	}

	for {
		for _, m := range mi {
			if path == m.Mountpoint {
				return m, nil
			}
		}

		if path == "/" {
			break
		}
		path = filepath.Dir(path)
	}

	return nil, fmt.Errorf("no matching mountpoint found")
}

// isDebugFSMounted would test the existence of file /sys/kernel/debug/tracing/kprobe_events to determine if debugfs (or debugfs AND tracefs) is/are mounted or not
// returns a boolean and a possible error message
func isDebugFSMounted() (bool, error) {
	_, err := os.Stat("/sys/kernel/debug/tracing/kprobe_events")
	if err != nil {
		if os.IsPermission(err) {
			return false, fmt.Errorf("eBPF not supported, does not have permission to access debugfs")
		} else if os.IsNotExist(err) {
			return false, fmt.Errorf("debugfs is not mounted and is needed for eBPF-based checks, run \"sudo mount -t debugfs none /sys/kernel/debug\" to mount debugfs")
		} else {
			return false, fmt.Errorf("an error occurred while accessing debugfs: %w", err)
		}
	}

	mi, err := GetMountPoint("/sys/kernel/debug/tracing/kprobe_events")
	if err != nil {
		return false, fmt.Errorf("unable to detect debugfs mount point: %w", err)
	}

	if mi.FSType != "tracefs" && mi.FSType != "debugfs" {
		return false, fmt.Errorf("kprobe_events mount point(%s): wrong fs type(%s)", mi.Mountpoint, mi.FSType)
	}

	// on fargate, kprobe_events is mounted as ro
	if !fargate.IsFargateInstance() {
		options := strings.Split(mi.Options, ",")
		for _, option := range options {
			if option == "ro" {
				return false, fmt.Errorf("kprobe_events mount point(%s) is in read-only mode", mi.Mountpoint)
			}
		}
	}

	return true, nil
}

// isTraceFSMounted would test the existence of file /sys/kernel/tracing/kprobe_events to determine if tracefs is mounted (and debugfs is not)
// returns a boolean and a possible error message
func isTraceFSMounted() bool {
	_, err := os.Stat("/sys/kernel/tracing/kprobe_events")
	if err != nil {
		if os.IsPermission(err) {
			seclog.Infof("eBPF not supported, does not have permission to access tracefs kprobe_events (/sys/kernel/tracing/kprobe_events)")
			return false
		} else if os.IsNotExist(err) {
			seclog.Infof("tracefs is not mounted in /sys/kernel/tracing/ and is needed for eBPF-based checks, run \"sudo mount -t tracefs none /sys/kernel/tracing\" to mount tracefs")
			return false
		} else {
			seclog.Infof("an error occurred while accessing tracefs sysfs entry (/sys/kernel/tracing/kprobe_events): %s", err)
			return false
		}
	}

	mi, err := GetMountPoint("/sys/kernel/tracing/kprobe_events")
	if err != nil {
		seclog.Infof("unable to detect tracefs mount point (/sys/kernel/tracing/): %s", err)
		return false
	}

	if mi.FSType != "tracefs" {
		seclog.Infof("kprobe_events mount point(%s): wrong fs type(%s)", mi.Mountpoint, mi.FSType)
		return false
	}

	// on fargate, kprobe_events is mounted as ro
	if !fargate.IsFargateInstance() {
		options := strings.Split(mi.Options, ",")
		for _, option := range options {
			if option == "ro" {
				seclog.Infof("tracefs mount point(%s) is in read-only mode", mi.Mountpoint)
				return false
			}
		}
	}

	return true
}

// Cf https://www.kernel.org/doc/Documentation/trace/ftrace.txt :
//     [...] When tracefs is configured into the kernel (which selecting any ftrace
//     option will do) the directory /sys/kernel/tracing will be created.
//     [...] Before 4.1, all ftrace tracing control files were within the debugfs
//     file system, which is typically located at /sys/kernel/debug/tracing.
//     For backward compatibility, when mounting the debugfs file system,
//     the tracefs file system will be automatically mounted at:
//      /sys/kernel/debug/tracing
//     All files located in the tracefs file system will be located in that
//     debugfs file system directory as well.

// IsDebugFSOrTraceFSMounted would test the existence of file /sys/kernel/tracing/kprobe_events to determine if tracefs is mounted or not
// returns a boolean and a possible error message
func IsDebugFSOrTraceFSMounted() (bool, error) {
	isMounted := isTraceFSMounted()
	if isMounted {
		return true, nil
	}
	return isDebugFSMounted()
}

func GetTraceFSMountPath() string {
	if isTraceFSMounted() {
		return "/sys/kernel/tracing"
	} else if isMounted, _ := isDebugFSMounted(); isMounted {
		return "/sys/kernel/debug/tracing"
	}
	return ""
}
