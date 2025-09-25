// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Installation context flags from Windows SDK (msi.h)
// See: https://learn.microsoft.com/en-us/windows/win32/msi/product-context
const (
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_USERMANAGED = 1
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_USERUNMANAGED = 2
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_MACHINE = 4
	//nolint:revive // Keep these constants in sync with the Windows SDK
	MSIINSTALLCONTEXT_ALL = 7
)

var (
	msi                           = windows.NewLazySystemDLL("msi.dll")
	procMsiSourceListAddSourceExW = msi.NewProc("MsiSourceListAddSourceExW")
	procMsiSourceListSetInfoW     = msi.NewProc("MsiSourceListSetInfoW")
	procMsiSourceListForceResExW  = msi.NewProc("MsiSourceListForceResolutionExW")
	procMsiEnumProductsExW        = msi.NewProc("MsiEnumProductsExW")
	procMsiGetProductInfoW        = msi.NewProc("MsiGetProductInfoW")
	procMsiEnumFeaturesW          = msi.NewProc("MsiEnumFeaturesW")
)

// MsiSourceListAddSourceEx adds or reorders the set of sources of a patch or product in a specified context.
// It can also create a source list for a patch that does not exist in the specified context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistaddsourceexw
func MsiSourceListAddSourceEx(productCode *uint16, context uint32, sourceType uint32, src *uint16, index uint32) windows.Errno {
	r1, _, _ := procMsiSourceListAddSourceExW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid (NULL for machine/current user)
		uintptr(context),                     // MSIINSTALLCONTEXT
		uintptr(sourceType),                  // dwOptions
		uintptr(unsafe.Pointer(src)),         // szSource (path only; no filename)
		uintptr(index),                       // dwIndex (1..N; 0 = append)
	)
	return windows.Errno(r1)
}

// MsiSourceListSetInfo sets information about the source list for a product or patch in a specific context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistsetinfow
func MsiSourceListSetInfo(productCode *uint16, context uint32, options uint32, propName *uint16, value *uint16) windows.Errno {
	r1, _, _ := procMsiSourceListSetInfoW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid
		uintptr(context),
		uintptr(options),                  // dwOptions (MSICODE_*)
		uintptr(unsafe.Pointer(propName)), // szProperty (e.g., "PackageName")
		uintptr(unsafe.Pointer(value)),    // szValue
	)
	return windows.Errno(r1)
}

// MsiSourceListForceResolutionEx removes the registration of the property called "LastUsedSource".
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msisourcelistforceresolutionexw
func MsiSourceListForceResolutionEx(productCode *uint16, context uint32, options uint32) windows.Errno {
	r1, _, _ := procMsiSourceListForceResExW.Call(
		uintptr(unsafe.Pointer(productCode)), // szProductCodeOrPatchCode
		0,                                    // szUserSid
		uintptr(context),
		uintptr(options), // dwOptions
	)
	return windows.Errno(r1)
}

// MsiEnumProductsEx  enumerates through one or all the instances of products that are
// currently advertised or installed in the specified contexts.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msienumproductsexw
func MsiEnumProductsEx(szProductCode *uint16, szUserSid *uint16, dwContext uint32, index uint32, productCodeBuf *uint16, pdwInstalledContext *uint32, sidBuf *uint16, sidLen *uint32) windows.Errno {
	ret, _, _ := procMsiEnumProductsExW.Call(
		uintptr(unsafe.Pointer(szProductCode)), // szProductCode = NULL
		uintptr(unsafe.Pointer(szUserSid)),     // szUserSid = NULL
		uintptr(dwContext),
		uintptr(index),
		uintptr(unsafe.Pointer(productCodeBuf)),
		uintptr(unsafe.Pointer(pdwInstalledContext)), // context
		uintptr(unsafe.Pointer(sidBuf)),
		uintptr(unsafe.Pointer(sidLen)),
	)
	return windows.Errno(ret)
}

// MsiGetProductInfo returns product information for advertised and installed products.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msigetproductinfoexw
func MsiGetProductInfo(productCode *uint16, propName *uint16, buf *uint16, bufLen *uint32) windows.Errno {
	ret, _, _ := procMsiGetProductInfoW.Call(
		uintptr(unsafe.Pointer(productCode)),
		uintptr(unsafe.Pointer(propName)),
		uintptr(unsafe.Pointer(buf)),
		uintptr(unsafe.Pointer(bufLen)),
	)
	return windows.Errno(ret)
}

// MsiEnumFeatures enumerates the published features for a given product.
//
// https://learn.microsoft.com/en-us/windows/win32/api/msi/nf-msi-msienumfeaturesw
func MsiEnumFeatures(productCode *uint16, index uint32, featureBuf *uint16, parentBuf *uint16) windows.Errno {
	ret, _, _ := procMsiEnumFeaturesW.Call(
		uintptr(unsafe.Pointer(productCode)),
		uintptr(index),
		uintptr(unsafe.Pointer(featureBuf)),
		uintptr(unsafe.Pointer(parentBuf)),
	)
	return windows.Errno(ret)
}

// EnumerateMsiProducts enumerates all the products in the specified context.
// It calls the processor function for each product found to get product information.
func EnumerateMsiProducts(dwContext uint32, processor func(productCode []uint16, context uint32, userSID string) error) error {
	// When making multiple calls to MsiEnumProducts to enumerate all the products, each call should be made from the same thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var index uint32
	for {
		var productCodeBuf [39]uint16
		var context uint32
		var sidBuf [256]uint16
		sidLen := uint32(len(sidBuf))

		ret := MsiEnumProductsEx(nil, nil, dwContext, index, &productCodeBuf[0], &context, &sidBuf[0], &sidLen)

		if errors.Is(ret, windows.ERROR_NO_MORE_ITEMS) {
			break
		}
		if !errors.Is(ret, windows.ERROR_SUCCESS) {
			return fmt.Errorf("error enumerating products at index %d: %d", index, ret)
		}

		userSID := ""
		if context == MSIINSTALLCONTEXT_USERMANAGED || context == MSIINSTALLCONTEXT_USERUNMANAGED {
			userSID = windows.UTF16ToString(sidBuf[:sidLen])
		}

		if err := processor(productCodeBuf[:], context, userSID); err != nil {
			return err
		}
		index++
	}
	return nil
}

// GetMsiProductInfo fetches a property from the MSI database.
func GetMsiProductInfo(propName string, productCode []uint16) (string, error) {
	bufLen := uint32(windows.MAX_PATH)
	ret := windows.ERROR_MORE_DATA
	for errors.Is(ret, windows.ERROR_MORE_DATA) {
		buf := make([]uint16, bufLen)
		propNamePtr, err := windows.UTF16PtrFromString(propName)
		if err != nil {
			return "", err
		}
		ret = MsiGetProductInfo(&productCode[0], propNamePtr, &buf[0], &bufLen)
		if errors.Is(ret, windows.ERROR_SUCCESS) {
			return windows.UTF16ToString(buf[:bufLen]), nil
		}
		bufLen++
	}
	return "", fmt.Errorf("unexpected return from msiGetProductInfo for %s: %w", propName, ret)
}
