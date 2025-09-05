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

	ret := winutil.MsiSourceListAddSourceEx(product.Code, msiInstallContextMachine, msiSourceTypeNetwork|msiCodeProduct, sourcePath, 1)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to add source to source list: %w", ret)
	}

	ret = winutil.MsiSourceListSetInfo(product.Code, msiInstallContextMachine, msiCodeProduct, "PackageName", packageName)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to set info for source list: %w", ret)
	}

	ret = winutil.MsiSourceListForceResolutionEx(product.Code, msiInstallContextMachine, msiCodeProduct)
	if !errors.Is(ret, windows.ERROR_SUCCESS) {
		return fmt.Errorf("failed to force resolution for source list: %w", ret)
	}

	return nil
}
