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

const datadogAgentProductName = "Datadog Agent"

func datadogProductRegistryKey(product string) string {
	return `SOFTWARE\Datadog\` + product
}

func openDatadogProductKeyRead(product string) (registry.Key, error) {
	return registry.OpenKey(registry.LOCAL_MACHINE, datadogProductRegistryKey(product), registry.READ)
}

// GetProgramDataDir returns the current programdatadir, usually
// c:\programdata\Datadog
func GetProgramDataDir() (path string, err error) {
	return GetProgramDataDirForProduct("Datadog Agent")
}

// GetProgramDataDirForProduct returns the current programdatadir, usually
// c:\programdata\Datadog given a product key name
func GetProgramDataDirForProduct(product string) (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return "", err
	}
	k, err := openDatadogProductKeyRead(product)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", datadogProductRegistryKey(product))
		return filepath.Join(res, "Datadog"), nil
	}
	defer k.Close()
	val, _, err := k.GetStringValue("ConfigRoot")
	if err != nil {
		log.Debugf("Windows installation key config not found, using default program data dir")
		return filepath.Join(res, "Datadog"), nil
	}
	path = val
	return
}

// GetProgramFilesDirForProduct returns the root of the installatoin directory,
// usually c:\program files\datadog\datadog agent
func GetProgramFilesDirForProduct(product string) (path string, err error) {
	res, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	k, err := openDatadogProductKeyRead(product)
	if err != nil {
		// if the key isn't there, we might be running a standalone binary that wasn't installed through MSI
		log.Debugf("Windows installation key root (%s) not found, using default program data dir", datadogProductRegistryKey(product))
		return filepath.Join(res, "Datadog", product), nil
	}
	defer k.Close()
	val, _, err := k.GetStringValue("InstallPath")
	if err != nil {
		log.Debugf("Windows installation key config not found, using default program data dir")
		return filepath.Join(res, "Datadog", product), nil
	}
	path = val
	return
}

// ReadFleetPoliciesDirFromRegistry reads HKLM\SOFTWARE\Datadog\Datadog Agent\fleet_policies_dir.
// It returns "" if the key or value is missing. pkg/config/setup.FleetConfigOverride uses this
// when fleet_policies_dir is unset in config; pkg/fleet/installer/paths uses it for installer-managed defaults.
func ReadFleetPoliciesDirFromRegistry() string {
	k, err := openDatadogProductKeyRead(datadogAgentProductName)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue("fleet_policies_dir")
	if err != nil || val == "" {
		return ""
	}
	return val
}
