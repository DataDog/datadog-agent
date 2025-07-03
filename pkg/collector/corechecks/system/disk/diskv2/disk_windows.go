// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package diskv2

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	win "golang.org/x/sys/windows"
	"slices"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	gopsutil_disk "github.com/shirou/gopsutil/v4/disk"
)

var defaultStatFn statFunc = func(_ string) (StatT, error) { return StatT{}, nil }

func defaultIgnoreCase() bool {
	return true
}

func baseDeviceName(device string) string {
	return strings.ToLower(strings.Trim(device, "\\"))
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
