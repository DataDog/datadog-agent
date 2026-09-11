// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DriverService is a kernel-mode or file-system driver registered with the Service Control
// Manager, the native replacement for the narrow projection of Win32_SystemDriver that the
// software inventory driver collector used to query over WMI.
type DriverService struct {
	// Name is the service name, e.g. "WdFilter". Windows guarantees it is unique on the
	// host, since it is the key name under HKLM\SYSTEM\CurrentControlSet\Services.
	Name string
	// DisplayName is the human-readable name, e.g. "Microsoft Defender Antivirus
	// Mini-Filter Driver". It may be empty, identical to Name, or an indirect resource
	// reference such as "@%SystemRoot%\System32\drivers\tcpipreg.sys,-10110".
	DisplayName string
	// ImagePath is the image path recorded for the service. It is not necessarily a Win32
	// path: it may be prefixed with \??\ or \SystemRoot\, or be relative to the Windows
	// directory. Empty when the per-driver QueryServiceConfig call fails.
	ImagePath string
}

// EnumDriverServices enumerates every kernel-mode and file-system driver service registered
// with the Service Control Manager.
//
// SERVICE_KERNEL_DRIVER|SERVICE_FILE_SYSTEM_DRIVER is passed rather than windows.SERVICE_DRIVER,
// because the latter also includes SERVICE_RECOGNIZER_DRIVER, which Win32_SystemDriver does not
// expose.
func EnumDriverServices() ([]DriverService, error) {
	// SC_MANAGER_CONNECT is required in addition to SC_MANAGER_ENUMERATE_SERVICE: the handle
	// is also used for a per-driver windows.OpenService below, and every other
	// enumerate-then-open call site in this repo (service.go's openManagerService,
	// pkg/flare/service_windows.go) requests both for exactly that reason.
	manager, err := OpenSCManager(windows.SC_MANAGER_CONNECT | windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return nil, fmt.Errorf("failed to open SCM: %w", err)
	}
	defer manager.Disconnect()

	const driverServiceType = windows.SERVICE_KERNEL_DRIVER | windows.SERVICE_FILE_SYSTEM_DRIVER

	var bytesNeeded, servicesReturned uint32
	var buf []byte
	for {
		var p *byte
		if len(buf) > 0 {
			p = &buf[0]
		}
		err = windows.EnumServicesStatusEx(manager.Handle, windows.SC_ENUM_PROCESS_INFO,
			driverServiceType, windows.SERVICE_STATE_ALL,
			p, uint32(len(buf)), &bytesNeeded, &servicesReturned, nil, nil)
		if err == nil {
			break
		}
		if err != windows.ERROR_MORE_DATA {
			return nil, fmt.Errorf("failed to enumerate driver services: %w", err)
		}
		if bytesNeeded <= uint32(len(buf)) {
			return nil, err
		}
		buf = make([]byte, bytesNeeded)
	}
	if servicesReturned == 0 {
		return nil, nil
	}

	services := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), servicesReturned)

	drivers := make([]DriverService, 0, len(services))
	for _, svc := range services {
		name := windows.UTF16PtrToString(svc.ServiceName)
		drivers = append(drivers, DriverService{
			Name:        name,
			DisplayName: windows.UTF16PtrToString(svc.DisplayName),
			ImagePath:   queryDriverImagePath(manager, name),
		})
	}

	return drivers, nil
}

// queryDriverImagePath opens name for SERVICE_QUERY_CONFIG and returns the BinaryPathName from
// its configuration.
//
// windows.QueryServiceConfig is called directly rather than through mgr.Service.Config():
// Config() adds two QueryServiceConfig2 round trips per driver, for Description and
// DelayedAutoStart, that nothing here reads. A failure to open or query the service is not
// fatal: it leaves ImagePath empty and logs at debug level, which reproduces a NULL
// Win32_SystemDriver.PathName, a case the collector already turns into a warning-and-skip.
func queryDriverImagePath(manager *mgr.Mgr, name string) string {
	handle, err := windows.OpenService(manager.Handle, windows.StringToUTF16Ptr(name), windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		log.Debugf("failed to open driver service %q: %v", name, err)
		return ""
	}
	defer func() {
		if closeErr := windows.CloseServiceHandle(handle); closeErr != nil {
			log.Debugf("failed to close handle for driver service %q: %v", name, closeErr)
		}
	}()

	// The buffer is []uint64, not []byte: QUERY_SERVICE_CONFIG holds *uint16 fields, which
	// require 8-byte alignment on amd64, and Go only guarantees that alignment for the backing
	// array of a []uint64 -- a []byte's first element is not provably aligned for a pointer-
	// containing struct. Casting a []byte buffer this way fails checkptr under race builds
	// ("misaligned pointer conversion") and can crash the real QueryServiceConfigW call with an
	// access violation otherwise, both observed in CI.
	const initialWords = 4096 / 8
	words := initialWords
	for {
		buf := make([]uint64, words)
		bufSize := uint32(words) * 8
		var bytesNeeded uint32
		config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
		err = windows.QueryServiceConfig(handle, config, bufSize, &bytesNeeded)
		if err == nil {
			return windows.UTF16PtrToString(config.BinaryPathName)
		}
		if err != windows.ERROR_INSUFFICIENT_BUFFER || bytesNeeded <= bufSize {
			log.Debugf("failed to query config for driver service %q: %v", name, err)
			return ""
		}
		words = int((bytesNeeded + 7) / 8)
	}
}
