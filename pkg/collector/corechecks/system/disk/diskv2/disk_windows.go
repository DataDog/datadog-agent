// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2

import (
    "fmt"
    "strings"
    "unsafe"

    win "golang.org/x/sys/windows"

    "github.com/DataDog/datadog-agent/pkg/aggregator/sender"
    "github.com/DataDog/datadog-agent/pkg/util/log"
    gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
)

var defaultStatFn statFunc = func(_ string) (StatT, error) { return StatT{}, nil }

// GetDriveTypeFn returns the Windows drive type for a given path.
// It is a variable so it can be overridden in tests.
var GetDriveTypeFn = func(path string) uint32 {
    typePath, err := win.UTF16PtrFromString(path)
    if err != nil {
        return win.DRIVE_UNKNOWN
    }
    return win.GetDriveType(typePath)
}

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
    // Skip CD-ROM drives entirely (with or without disk inserted).
    // The Python check (via psutil) used GetDriveType() to skip DRIVE_CDROM.
    // gopsutil does not expose drive type in PartitionStat, so we call
    // GetDriveType() directly. This avoids monitoring CD-ROM drives that
    // may cause ENOENT, pop-up Windows GUI errors, or just hang.
    if GetDriveTypeFn(partition.Mountpoint) == win.DRIVE_CDROM {
        return true
    }
    // Also skip drives with no filesystem (not ready / no media)
    return partition.Fstype == ""
}

var mpr = win.NewLazySystemDLL("mpr.dll")
var procWNetAddConnection2W = mpr.NewProc("WNetAddConnection2W")

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

func (c *Check) sendInodesMetrics(_ sender.Sender, _ *gopsutil_disk.UsageStat, _ []string) {
}

func (c *Check) loadRootDevices() (map[string]string, error) {
    rootDevices := make(map[string]string)

    return rootDevices, nil
}
