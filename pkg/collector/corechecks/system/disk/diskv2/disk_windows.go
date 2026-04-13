// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/benbjohnson/clock"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
	win "golang.org/x/sys/windows"
)

var defaultStatFn statFunc = func(_ string) (StatT, error) { return StatT{}, nil }

func defaultIgnoreCase() bool {
	return true
}

func baseDeviceName(device string) string {
	return strings.ToLower(strings.Trim(device, "\\"))
}

// normalizeDeviceTag returns the device name for use in the device: tag.
// On Windows, strips backslashes and lowercases (legacy behavior for C:\\ -> c:).
func normalizeDeviceTag(deviceName string) string {
	return strings.ToLower(strings.Trim(deviceName, "\\"))
}

func (c *Check) fetchAllDeviceLabelsFromLsblk() error {
	return nil
}

func (c *Check) fetchAllDeviceLabelsFromBlkidCache() error {
	return nil
}

func (c *Check) fetchAllDeviceLabelsFromBlkid() error {
	return nil
}

func (c *Check) excludePartitionInPlatform(partition gopsutil_disk.PartitionStat) bool {
	/* skip cd-rom drives with no disk in it; they may raise
	ENOENT, pop-up a Windows GUI error for a non-ready
	partition or just hang;
	and all the other excluded disks */
	return slices.Contains(partition.Opts, "cdrom") || partition.Fstype == ""
}

var mpr = win.NewLazySystemDLL("mpr.dll")
var procWNetAddConnection2W = mpr.NewProc("WNetAddConnection2W")

var modkernel32 = win.NewLazySystemDLL("kernel32.dll")
var procCancelSynchronousIO = modkernel32.NewProc("CancelSynchronousIo")

type netResource struct {
	Scope       uint32
	Type        uint32
	DisplayType uint32
	Usage       uint32
	localName   *uint16
	remoteName  *uint16
	comment     *uint16
	provider    *uint16
}

// RemotePath constructs the remote path based on the mount type.
// It converts the Type to uppercase to ensure case-insensitive evaluation.
func remotePath(m mount) string {
	if strings.TrimSpace(m.Type) == "" {
		m.Type = "smb"
	}
	// Convert Type to uppercase for case-insensitive comparison
	normalizedType := strings.ToLower(strings.TrimSpace(m.Type))
	if normalizedType == "nfs" {
		return fmt.Sprintf(`%s:%s`, m.Host, m.Share)
	}
	return fmt.Sprintf(`\\%s\%s`, m.Host, m.Share)
}

func (c *Check) configureCreateMounts() {
	for _, m := range c.instanceConfig.CreateMounts {
		if len(m.Host) == 0 || len(m.Share) == 0 {
			log.Errorf("Invalid configuration. Drive mount requires remote machine and share point")
			continue
		}
		log.Debugf("Mounting: %s\n", m)
		remoteName := remotePath(m)
		err := NetAddConnection(m.MountPoint, remoteName, m.Password, m.User)
		if err != nil {
			log.Errorf("Failed to mount %s on %s: %s", m.MountPoint, remoteName, err)
			continue
		}
		log.Debugf("Successfully mounted %s as %s\n", m.MountPoint, remoteName)
	}
}

// NetAddConnection specifies the command used to add a new network connection.
var NetAddConnection = func(localName, remoteName, password, username string) error {
	return wNetAddConnection2(localName, remoteName, password, username)
}

func createNetResource(localName, remoteName string) (netResource, error) {
	lpLocalName, err := win.UTF16PtrFromString(localName)
	if err != nil {
		return netResource{}, fmt.Errorf("failed to convert local name to UTF16: %w", err)
	}
	lpRemoteName, err := win.UTF16PtrFromString(remoteName)
	if err != nil {
		return netResource{}, fmt.Errorf("failed to convert remote name to UTF16: %w", err)
	}
	return netResource{
		localName:  lpLocalName,
		remoteName: lpRemoteName,
	}, nil
}

func wNetAddConnection2(localName, remoteName, password, username string) error {
	netResource, err := createNetResource(localName, remoteName)
	if err != nil {
		return fmt.Errorf("failed to create NetResource: %w", err)
	}
	var _password *uint16
	if password == "" {
		_password = nil
	} else {
		_password, err = win.UTF16PtrFromString(password)
		if err != nil {
			return fmt.Errorf("failed to convert password to UTF16: %w", err)
		}
	}
	var _username *uint16
	if username == "" {
		_username = nil
	} else {
		_username, err = win.UTF16PtrFromString(username)
		if err != nil {
			return fmt.Errorf("failed to convert username to UTF16: %w", err)
		}
	}
	rc, _, err := procWNetAddConnection2W.Call(
		uintptr(unsafe.Pointer(&netResource)),
		uintptr(unsafe.Pointer(_password)),
		uintptr(unsafe.Pointer(_username)),
		0,
	)
	if rc != 0 {
		return err
	}
	return nil
}

// isExpectedIOCounterError returns true for Windows errors that indicate the
// system does not support IOCTL_DISK_PERFORMANCE (e.g. disk performance
// counters disabled on Windows Server 2016, or virtual drives like Google Drive).
func isExpectedIOCounterError(err error) bool {
	return errors.Is(err, win.ERROR_INVALID_FUNCTION) || errors.Is(err, win.ERROR_NOT_SUPPORTED)
}

func (c *Check) sendInodesMetrics(_ sender.Sender, _ *gopsutil_disk.UsageStat, _ []string) {
}

func (c *Check) loadRootDevices() (map[string]string, error) {
	rootDevices := make(map[string]string)

	return rootDevices, nil
}

// diskUsageInterruptible runs fn(mountpoint) on an OS-thread-locked goroutine and
// waits up to timeout for it to complete. On timeout, CancelSynchronousIo is called
// on the goroutine's OS thread, which causes the in-progress SMB/NFS I/O to return
// with ERROR_OPERATION_ABORTED instead of blocking indefinitely.
func diskUsageInterruptible(fn func(string) (*gopsutil_disk.UsageStat, error), mountpoint string, timeout time.Duration, clk clock.Clock) (*gopsutil_disk.UsageStat, error) {
	type result struct {
		usage *gopsutil_disk.UsageStat
		err   error
	}

	resultCh := make(chan result, 1)
	// Buffered so the goroutine can always send its handle without blocking,
	// even if the main goroutine has already moved past the receive.
	handleCh := make(chan win.Handle, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// GetCurrentThread() returns a pseudo-handle only valid on this thread.
		// OpenThread() gives a real handle usable from other goroutines.
		tid := win.GetCurrentThreadId()
		handle, err := win.OpenThread(win.THREAD_TERMINATE, false, tid)
		if err != nil {
			log.Debugf("disk usage: failed to open thread handle, I/O cancellation unavailable: %v", err)
			handle = win.Handle(0)
		}
		handleCh <- handle

		usage, err := fn(mountpoint)
		resultCh <- result{usage, err}
	}()

	// Wait for the goroutine to be pinned to its OS thread and have its handle ready.
	// This is fast (goroutine startup + one Win32 syscall).
	handle := <-handleCh
	defer func() {
		if handle != win.Handle(0) {
			win.CloseHandle(handle)
		}
	}()

	select {
	case r := <-resultCh:
		return r.usage, r.err
	case <-clk.After(timeout):
		if handle != win.Handle(0) {
			// Interrupt the blocking I/O on the goroutine's OS thread.
			// The fn() call returns with ERROR_OPERATION_ABORTED.
			// The goroutine then sends to the buffered resultCh and exits cleanly.
			procCancelSynchronousIO.Call(uintptr(handle))
		}
		return nil, fmt.Errorf("disk usage call timed out after %s", timeout)
	}
}
