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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func GetProgramDataDirForProduct(product string) (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		// Something is terribly wrong on the system if %PROGRAMDATA% is missing
		return "", err
	}
	keyname := "SOFTWARE\\Datadog\\" + product
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		keyname,
		registry.ALL_ACCESS)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", keyname)
		return filepath.Join(res, product), nil
	}
	defer k.Close()
	val, _, err := k.GetStringValue("ConfigRoot")
	if err != nil {
		log.Warnf("Windows installation key config not found, using default program data dir")
		return filepath.Join(res, product), nil
	}
	path = val
	return
}
