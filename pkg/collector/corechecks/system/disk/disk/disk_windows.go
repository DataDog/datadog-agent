// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

//nolint:revive // TODO(AGENTRUN) Fix revive linter
package disk

import (
	"fmt"
	win "golang.org/x/sys/windows"
	"unsafe"
)

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

type NetResource struct {
	Scope       uint32
	Type        uint32
	DisplayType uint32
	Usage       uint32
	localName   *uint16
	remoteName  *uint16
	comment     *uint16
	provider    *uint16
}

var NetAddConnection = func(mountType, localName, remoteName, password, username string) error {
	return WNetAddConnection2(localName, remoteName, password, username)
}

func createNetResource(localName, remoteName string) (NetResource, error) {
	lpLocalName, err := win.UTF16PtrFromString(localName)
	if err != nil {
		return NetResource{}, fmt.Errorf("failed to convert local name to UTF16: %w", err)
	}
	lpRemoteName, err := win.UTF16PtrFromString(remoteName)
	if err != nil {
		return NetResource{}, fmt.Errorf("failed to convert remote name to UTF16: %w", err)
	}
	return NetResource{
		localName:  lpLocalName,
		remoteName: lpRemoteName,
	}, nil
}

func WNetAddConnection2(localName, remoteName, password, username string) error {
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
