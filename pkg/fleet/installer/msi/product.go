// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
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
func RemoveProduct(ctx context.Context, productName string, opts ...MsiexecOption) error {
	options := []MsiexecOption{
		Uninstall(),
		WithProduct(productName),
	}
	options = append(options, opts...)
	cmd, err := Cmd(options...)
	if err != nil {
		return fmt.Errorf("failed to remove product: %w", err)
	}
	err = cmd.Run(ctx)
	if err != nil {
		err = fmt.Errorf("failed to remove product: %w", err)
		var msiErr *MsiexecError
		if errors.As(err, &msiErr) {
			err = fmt.Errorf("%w\n%s", err, msiErr.ProcessedLog)
		}
		return err
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

// FindAllProductCodes looks for all products with the given productName using the Windows Installer API
// It enumerates through all products and checks if the product name matches the given productName.
func FindAllProductCodes(productName string) ([]string, error) {
	// When making multiple calls to MsiEnumProducts to enumerate all the products, each call should be made from the same thread.
	// runtime.LockOSThread()
	// defer runtime.UnlockOSThread()

	// var index uint32
	var productCodes []string

	err := winutil.EnumerateMsiProducts(winutil.MSIINSTALLCONTEXT_MACHINE, func(productCode []uint16, context uint32, userSID string) error {
		// Get display name and check if it matches
		displayName, err := winutil.GetProp("ProductName", productCode)
		if err != nil {
			return err // or continue with warning
		}
		if displayName == productName {
			productCodes = append(productCodes, windows.UTF16ToString(productCode[:]))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(productCodes) == 0 {
		return nil, fmt.Errorf("no products found")
	}

	return productCodes, nil
}
