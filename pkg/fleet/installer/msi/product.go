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
	// Features is a list of features installed by the product.
	Features []string
}

// FindProductCode finds the first product with the specified display name
func FindProductCode(productName string) (*Product, error) {
	products, err := FindUninstallProductCodes(productName)
	if err != nil {
		return nil, err
	}
	return products[0], nil
}

// FindUninstallProductCodes looks for the productName in the registry and returns information about it
func FindUninstallProductCodes(productName string) ([]*Product, error) {
	var products []*Product
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
			products = append(products, product)
		}
	}
	if len(products) == 0 {
		return nil, fmt.Errorf("no products found with name: %s", productName)
	}
	return products, nil
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
func FindAllProductCodes(productName string) ([]Product, error) {
	var products []Product

	err := winutil.EnumerateMsiProducts(winutil.MSIINSTALLCONTEXT_MACHINE, func(productCode []uint16, _ uint32, _ string) error {
		// Get display name and check if it matches
		displayName, err := winutil.GetMsiProductInfo("ProductName", productCode)
		if err != nil {
			return err // or continue with warning
		}
		if displayName == productName {
			features, err := GetProductFeatures(productCode)
			if err != nil {
				features = append(features, fmt.Sprintf("error getting features: %v", err))
			}
			product := Product{
				Code:     windows.UTF16ToString(productCode[:]),
				Features: features,
			}
			products = append(products, product)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(products) == 0 {
		return nil, errors.New("no products found")
	}

	return products, nil
}

// GetProductFeatures enumberates all features for a given product code and returns them as a list of strings.
func GetProductFeatures(productCode []uint16) ([]string, error) {
	var features []string
	var index uint32
	bufferSize := uint32(windows.MAX_PATH)

	for {
		featureBuf := make([]uint16, bufferSize)
		parentBuf := make([]uint16, bufferSize)

		ret := winutil.MsiEnumFeatures(&productCode[0], index, &featureBuf[0], &parentBuf[0])

		if errors.Is(ret, windows.ERROR_NO_MORE_ITEMS) {
			break
		}
		if errors.Is(ret, windows.ERROR_MORE_DATA) {
			bufferSize++
			continue
		}
		if !errors.Is(ret, windows.ERROR_SUCCESS) {
			return nil, fmt.Errorf("error enumerating features: %w", ret)
		}

		// Just use UTF16ToString which will find the null terminator automatically
		// This ignores the potentially corrupted bufLen value
		feature := windows.UTF16ToString(featureBuf)
		if feature != "" { // Only add non-empty features
			features = append(features, feature)
		}
		index++
	}

	return features, nil
}
