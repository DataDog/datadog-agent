// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package winutil

import (
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getDefaultProgramDataDir() (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err == nil {
		path = filepath.Join(res, "Datadog")
	}
	return
}

// GetProgramDataDir returns the current programdatadir, usually
// c:\programdata\Datadog
func GetProgramDataDir() (path string, err error) {
	return GetProgramDataDirForProduct("Datadog Agent")
}

// GetProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func GetProgramDataDirForProduct(product string) (path string, err error) {
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return getDefaultProgramDataDir()
	}
	defer k.Close()
	val, _, err := k.GetStringValue("ConfigRoot")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return getDefaultProgramDataDir()
	}
	path = val
	return
}

// GetProgramFilesDirForProduct returns the root of the installatoin directory,
// usually c:\program files\datadog\datadog agent
func GetProgramFilesDirForProduct(product string) (path string, err error) {
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return getDefaultProgramFilesDir()
	}
	defer k.Close()
	val, _, err := k.GetStringValue("InstallPath")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return getDefaultProgramFilesDir()
	}
	path = val
	return
}

func getDefaultProgramFilesDir() (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err == nil {
		path = filepath.Join(res, "Datadog", "Datadog Agent")
	}
	return
}
