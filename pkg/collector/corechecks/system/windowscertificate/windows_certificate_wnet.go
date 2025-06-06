// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package windowscertificate implements a windows certificate check
package windowscertificate

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var mpr = windows.NewLazySystemDLL("mpr.dll")
var procWNetAddConnection2W = mpr.NewProc("WNetAddConnection2W")
var procWNetCancelConnection2W = mpr.NewProc("WNetCancelConnection2W")

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

func createNetResource(remoteName, localName string) (netResource, error) {
	lpRemoteName, err := windows.UTF16PtrFromString(remoteName)
	if err != nil {
		return netResource{}, fmt.Errorf("failed to convert remote name to UTF16: %w", err)
	}
	lpLocalName, err := windows.UTF16PtrFromString(localName)
	if err != nil {
		return netResource{}, fmt.Errorf("failed to convert local name to UTF16: %w", err)
	}
	return netResource{
		remoteName: lpRemoteName,
		localName:  lpLocalName,
	}, nil
}

func wNetAddConnection2(remoteName, localName, password, username string) error {
	netResource, err := createNetResource(remoteName, localName)
	if err != nil {
		return fmt.Errorf("failed to create NetResource: %w", err)
	}
	var _password *uint16
	if password == "" {
		_password = nil
	} else {
		_password, err = windows.UTF16PtrFromString(password)
		if err != nil {
			return fmt.Errorf("failed to convert password to UTF16: %w", err)
		}
	}
	var _username *uint16
	if username == "" {
		_username = nil
	} else {
		_username, err = windows.UTF16PtrFromString(username)
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

func wNetCancelConnection2(name string) error {
	cname, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("failed to convert name to UTF16: %w", err)
	}

	ret, _, _ := procWNetCancelConnection2W.Call(
		uintptr(unsafe.Pointer(cname)),
		0,
		1,
	)

	if ret != 0 {
		return fmt.Errorf("WNetCancelConnection2W failed with code %d", ret)
	}

	return nil
}
