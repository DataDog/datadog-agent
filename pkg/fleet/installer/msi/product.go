// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	installproperty_localpackage = "LocalPackage"
)

var (
	msiDLL                 = syscall.NewLazyDLL("msi.dll")
	procMsiGetProductInfoW = msiDLL.NewProc("MsiGetProductInfoW")
)

// Product represents a software from the Windows Registry
type Product struct {
	// Code is the software product code
	Code string
	// UninstallString is the string that can be executed to uninstall the software. May be empty.
	UninstallString string
}

// FindProductCode looks for the productName in the registry and returns information about it
func FindProductCode(productName string) (*Product, error) {
	rootPath := "SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall"
	reg, err := registry.OpenKey(registry.LOCAL_MACHINE, rootPath, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, err
	}
	defer reg.Close()
	keys, err := reg.ReadSubKeyNames(0)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		product, err := processKey(rootPath, key, productName)
		if err == nil && product != nil {
			return product, nil
		}
	}
	return nil, fmt.Errorf("product not found")
}

// IsProductInstalled returns true if the given productName is installed
func IsProductInstalled(productName string) bool {
	product, err := FindProductCode(productName)
	if err != nil {
		return false
	}
	return product != nil
}

// RemoveProduct uses the registry to try and find a product and use msiexec to remove it.
// It is different from msiexec in that it uses the registry and not the stable/experiment path on disk to
// uninstall the product.
// This is needed because in certain circumstances the installer database stored in the stable/experiment paths does not
// reflect the installed version, and using those installers can lead to undefined behavior (either failure to uninstall,
// or weird bugs from uninstalling a product with an installer from a different version).
func RemoveProduct(productName string) error {
	cmd, err := Cmd(Uninstall(), WithProduct(productName))
	if err != nil {
		return fmt.Errorf("failed to remove product: %w", err)
	}
	output, err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to remove product: %w\n%s", err, string(output))
	}
	return nil
}

func processKey(rootPath, key, name string) (*Product, error) {
	subkey, err := registry.OpenKey(registry.LOCAL_MACHINE, rootPath+"\\"+key, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer subkey.Close()

	displayName, _, err := subkey.GetStringValue("DisplayName")
	if err == nil && displayName == name {
		product := &Product{}
		product.UninstallString, _, _ = subkey.GetStringValue("UninstallString")
		product.Code = key
		return product, nil
	}

	return nil, nil
}

// FindProductMSI returns the path to the MSI file for the given product code
func FindProductMSI(productCode string) (string, error) {
	return msiGetProductInfo(productCode, installproperty_localpackage)
}

// GetProductVersion returns the version of the product with the given product code
func GetProductVersion(productCode string) (string, error) {
	return msiGetProductInfo(productCode, "VersionString")
}

// MsiGetProductInfo retrieves the requested information about a product in the Windows Installer database
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msigetproductinfoa
func msiGetProductInfo(productCode, property string) (string, error) {
	var value [syscall.MAX_PATH]uint16
	var size uint32 = syscall.MAX_PATH

	productCodePtr, err := syscall.UTF16PtrFromString(productCode)
	if err != nil {
		return "", err
	}

	propertyPtr, err := syscall.UTF16PtrFromString(property)
	if err != nil {
		return "", err
	}

	r, _, err := procMsiGetProductInfoW.Call(
		uintptr(unsafe.Pointer(productCodePtr)),
		uintptr(unsafe.Pointer(propertyPtr)),
		uintptr(unsafe.Pointer(&value[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	var windowsErr = windows.Errno(r)
	if windowsErr != windows.ERROR_SUCCESS {
		return "", fmt.Errorf("MsiGetProductInfo failed: %v", err)
	}

	return syscall.UTF16ToString(value[:]), nil
}
