// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	mpr                        = windows.NewLazySystemDLL("mpr.dll")
	procWNetAddConnection2W    = mpr.NewProc("WNetAddConnection2W")
	procWNetCancelConnection2W = mpr.NewProc("WNetCancelConnection2W")
)

// NetResource contains information about a network resource
type NetResource struct {
	Scope       uint32
	Type        uint32
	DisplayType uint32
	Usage       uint32
	LocalName   *uint16
	RemoteName  *uint16
	Comment     *uint16
	Provider    *uint16
}

// CreateNetResource creates a netresource struct
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnetwk/ns-winnetwk-netresourcew
func CreateNetResource(remoteName, localName, comment, provider string, scope, resourceType, displayType, usage uint32) (NetResource, error) {

	var lpRemoteName *uint16
	var lpLocalName *uint16
	var lpComment *uint16
	var lpProvider *uint16
	var err error

	if remoteName != "" {
		lpRemoteName, err = windows.UTF16PtrFromString(remoteName)
		if err != nil {
			return NetResource{}, fmt.Errorf("failed to convert remote name to UTF16: %w", err)
		}
	}
	if localName != "" {
		lpLocalName, err = windows.UTF16PtrFromString(localName)
		if err != nil {
			return NetResource{}, fmt.Errorf("failed to convert local name to UTF16: %w", err)
		}
	}
	if comment != "" {
		lpComment, err = windows.UTF16PtrFromString(comment)
		if err != nil {
			return NetResource{}, fmt.Errorf("failed to convert comment to UTF16: %w", err)
		}
	}
	if provider != "" {
		lpProvider, err = windows.UTF16PtrFromString(provider)
		if err != nil {
			return NetResource{}, fmt.Errorf("failed to convert provider to UTF16: %w", err)
		}
	}
	return NetResource{
		Scope:       scope,
		Type:        resourceType,
		DisplayType: displayType,
		Usage:       usage,
		RemoteName:  lpRemoteName,
		LocalName:   lpLocalName,
		Comment:     lpComment,
		Provider:    lpProvider,
	}, nil
}

// WNetAddConnection2 makes a connection to a network resource and can redirect a local device to the network resource
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnetwk/nf-winnetwk-wnetaddconnection2w
func WNetAddConnection2(netResource *NetResource, password, username string, flags uint32) error {
	var _username *uint16
	var _password *uint16
	var err error

	if password == "" {
		_password = nil
	} else {
		_password, err = windows.UTF16PtrFromString(password)
		if err != nil {
			return fmt.Errorf("failed to convert password to UTF16: %w", err)
		}
	}
	if username == "" {
		_username = nil
	} else {
		_username, err = windows.UTF16PtrFromString(username)
		if err != nil {
			return fmt.Errorf("failed to convert username to UTF16: %w", err)
		}
	}
	rc, _, err := procWNetAddConnection2W.Call(
		uintptr(unsafe.Pointer(netResource)),
		uintptr(unsafe.Pointer(_password)),
		uintptr(unsafe.Pointer(_username)),
		uintptr(flags),
	)
	if rc != windows.NO_ERROR {
		return err
	}
	return nil
}

// WNetCancelConnection2 cancels an existing network connection.
//
// https://learn.microsoft.com/en-us/windows/win32/api/winnetwk/nf-winnetwk-wnetcancelconnection2w
func WNetCancelConnection2(name string) error {
	cname, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("failed to convert name to UTF16: %w", err)
	}

	ret, _, _ := procWNetCancelConnection2W.Call(
		uintptr(unsafe.Pointer(cname)),
		0,
		1,
	)

	if ret != windows.NO_ERROR {
		return fmt.Errorf("WNetCancelConnection2W failed with code %d", ret)
	}

	return nil
}
