// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package winregistry provides helper functions for interacting with the Windows Registry
package winregistry

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
)

// GetProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func GetProgramDataDirForProduct(product string) (path string, err error) {
	programData, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		// Something is terribly wrong on the system if %PROGRAMDATA% is missing
		return "", err
	}
	path, err = getDatadogKeyForProduct(product, "ConfigRoot")
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		return filepath.Join(programData, "Datadog"), nil
	}
	return
}

// GetProgramFilesDirForProduct returns the current ProgramFiles dir, usually
// c:\Program Files\Datadog\<product> given a product key name
func GetProgramFilesDirForProduct(product string) (path string, err error) {
	programFiles, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		// Something is terribly wrong on the system if %PROGRAMDATA% is missing
		return "", err
	}
	path, err = getDatadogKeyForProduct(product, "InstallPath")
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		return filepath.Join(programFiles, "Datadog", product), nil
	}
	return
}

func getDatadogKeyForProduct(product, key string) (path string, err error) {
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		return "", err
	}
	defer k.Close()
	val, _, err := k.GetStringValue(key)
	if err != nil {
		return "", err
	}
	path = val
	return
}
