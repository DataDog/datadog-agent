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
	"regexp"
	"strings"
	"unsafe"
)

func compileRegExp(expr string) (*regexp.Regexp, error) {
	iExpr := fmt.Sprintf("(?i)%s", expr)
	return regexp.Compile(iExpr)
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
func (m mount) RemotePath() (string, string) {
	if strings.TrimSpace(m.Type) == "" {
		m.Type = "SMB"
	}
	// Convert Type to uppercase for case-insensitive comparison
	normalizedType := strings.ToUpper(strings.TrimSpace(m.Type))
	if normalizedType == "NFS" {
		return normalizedType, fmt.Sprintf(`%s:%s`, m.Host, m.Share)
	}
	var userAndPassword string
	if len(m.User) > 0 {
		userAndPassword += m.User
	}
	if len(m.Password) > 0 {
		userAndPassword += fmt.Sprintf(":%s", m.Password)
	}
	if len(userAndPassword) > 0 {
		return normalizedType, fmt.Sprintf(`\\%s@%s\%s`, userAndPassword, m.Host, m.Share)
	}
	return normalizedType, fmt.Sprintf(`\\%s\%s`, m.Host, m.Share)
}

func (c *Check) configureCreateMounts() {
	for _, m := range c.instanceConfig.CreateMounts {
		if len(m.Host) == 0 || len(m.Share) == 0 {
			log.Errorf("Invalid configuration. Drive mount requires remote machine and share point")
			continue
		}
		log.Debugf("Mounting: %s\n", m)
		mountType, remoteName := m.RemotePath()
		log.Debugf("mountType: %s\n", mountType)
		err := NetAddConnection(mountType, m.MountPoint, remoteName, m.Password, m.User)
		if err != nil {
			log.Errorf("Failed to mount %s on %s: %s", m.MountPoint, remoteName, err)
			continue
		}
		log.Debugf("Successfully mounted %s as %s\n", m.MountPoint, remoteName)
	}
}

// NetAddConnection specifies the command used to add a new network connection.
var NetAddConnection = func(_mountType, localName, remoteName, password, username string) error {
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
