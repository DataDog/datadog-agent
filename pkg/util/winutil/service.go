// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package winutil

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

type ServiceManager struct {
	manager *mgr.Mgr
	service *mgr.Service
}

func (sm *ServiceManager) Connect(desiredAccess uint32) error {
	h, err := windows.OpenSCManager(nil, nil, desiredAccess)
	if err != nil {
		return err
	}
	sm.manager = &mgr.Mgr{Handle: h}
	return nil
}

func (sm *ServiceManager) OpenService(serviceName string, desiredAccess uint32) error {
	h, err := windows.OpenService(sm.manager.Handle, windows.StringToUTF16Ptr(serviceName), desiredAccess)
	if err != nil {
		return err
	}
	sm.service = &mgr.Service{Name: serviceName, Handle: h}
	return err
}
