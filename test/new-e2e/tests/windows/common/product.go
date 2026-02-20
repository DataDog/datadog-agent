// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// GetProductCodeByName returns the product code GUID for the given product name
func GetProductCodeByName(host *components.RemoteHost, name string) (string, error) {
	// Read from registry instead of using Win32_Product, which has negative side effects
	// https://gregramsey.net/2012/02/20/win32_product-is-evil/
	cmd := fmt.Sprintf(`(@(Get-ChildItem -Path "HKLM:SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall" -Recurse ; Get-ChildItem -Path "HKLM:SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall" -Recurse ) | Where {$_.GetValue("DisplayName") -like "%s" }).PSChildName`, name)
	val, err := host.Execute(cmd)
	if err != nil {
		fmt.Println(val)
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "", fmt.Errorf("product '%s' not found", name)
	}
	return val, nil
}

// GetProductVersionByName returns the product version for the given product name
// Pulls version from MSI registry infomration
func GetProductVersionByName(host *components.RemoteHost, name string) (string, error) {
	// get GUID
	guid, err := GetProductCodeByName(host, name)
	if err != nil {
		return "", err
	}
	// get verion string
	cmd := fmt.Sprintf(`(Get-ItemProperty -Path "HKLM:SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\%s").DisplayVersion`, guid)
	val, err := host.Execute(cmd)
	if err != nil {
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return "", errors.New("display version not found")
	}
	return val, nil
}
