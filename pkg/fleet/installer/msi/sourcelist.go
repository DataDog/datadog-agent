// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	msiInstallContextMachine = 4
	msiSourceTypeNetwork     = 0x00000001
	msiCodeProduct           = 0x00000000
)

// SetSourceList sets the source list for a given product name
func SetSourceList(productName string, sourcePath string, packageName string) error {
	product, err := FindProductCode(productName)
	if err != nil {
		return err
	}

	sourcePathPtr, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to convert source path to UTF16: %w", err)
	}

	productCodePtr, err := windows.UTF16PtrFromString(product.Code)
	if err != nil {
		return fmt.Errorf("failed to convert product code to UTF16: %w", err)
	}

	packageNamePtr, err := windows.UTF16PtrFromString(packageName)
	if err != nil {
		return fmt.Errorf("failed to convert package name to UTF16: %w", err)
	}

	propNamePtr, err := windows.UTF16PtrFromString("PackageName")
	if err != nil {
		return fmt.Errorf("failed to convert property name to UTF16: %w", err)
	}

	ret := winutil.MsiSourceListAddSourceEx(productCodePtr, msiInstallContextMachine, msiSourceTypeNetwork|msiCodeProduct, sourcePathPtr, 1)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to add source to source list: %w", ret)
	}

	ret = winutil.MsiSourceListSetInfo(productCodePtr, msiInstallContextMachine, msiCodeProduct, propNamePtr, packageNamePtr)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to set info for source list: %w", ret)
	}

	ret = winutil.MsiSourceListForceResolutionEx(productCodePtr, msiInstallContextMachine, msiCodeProduct)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to force resolution for source list: %w", ret)
	}

	return nil
}
