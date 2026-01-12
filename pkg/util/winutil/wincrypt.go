// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package winutil contains Windows OS utilities
package winutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32                               = windows.NewLazySystemDLL("crypt32.dll")
	procCertEnumCRLsInStore               = crypt32.NewProc("CertEnumCRLsInStore")
	procCertFreeCRLContext                = crypt32.NewProc("CertFreeCRLContext")
	procCertGetCRLContextProperty         = crypt32.NewProc("CertGetCRLContextProperty")
	procCertGetCertificateContextProperty = crypt32.NewProc("CertGetCertificateContextProperty")
	procCertNameToStr                     = crypt32.NewProc("CertNameToStrW")
)

// CRLContext contains both the encoded and decoded representations of a certificate revocation list (CRL).
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/ns-wincrypt-crl_context
type CRLContext struct {
	DwCertEncodingType uint32
	PbCrlEncoded       *byte
	CbCrlEncoded       uint32
	PCrlInfo           *CRLInfo
	HCertStore         windows.Handle
}

// CRLInfo contains the information of a certificate revocation list (CRL).
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/ns-wincrypt-crl_info
type CRLInfo struct {
	dwVersion          uint32 // nolint:unused
	SignatureAlgorithm windows.CryptAlgorithmIdentifier
	Issuer             windows.CertNameBlob
	ThisUpdate         windows.Filetime
	NextUpdate         windows.Filetime
	cCRLEntry          uint32                   // nolint:unused
	rgCRLEntry         []*CRLEntry              // nolint:unused
	cExtension         uint32                   // nolint:unused
	rgExtension        []*windows.CertExtension // nolint:unused
}

// CRLEntry contains information about a single revoked certificate. It is a member of the CRLInfo struct.
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/ns-wincrypt-crl_entry
type CRLEntry struct {
	SerialNumber   windows.CryptIntegerBlob
	RevocationDate windows.Filetime
	cExtension     uint32                   // nolint:unused
	rgExtension    []*windows.CertExtension // nolint:unused
}

// CertEnumCRLsInStore retrieves the first or next certificate revocation list (CRL) context in a certificate store.
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certenumcrlsinstore
func CertEnumCRLsInStore(hCertStore windows.Handle, pPrevCrlContext *CRLContext) (*CRLContext, error) {
	r0, _, err := procCertEnumCRLsInStore.Call(uintptr(hCertStore), uintptr(unsafe.Pointer(pPrevCrlContext)))
	crlcontext := (*CRLContext)(unsafe.Pointer(r0)) //nolint:govet
	if crlcontext == nil {
		return nil, err
	}
	return crlcontext, nil
}

// CertFreeCRLContext frees a certificate revocation list (CRL) context by decrementing its reference count.
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certfreecrlcontext
func CertFreeCRLContext(pCrlContext *CRLContext) error {
	r1, _, err := procCertFreeCRLContext.Call(uintptr(unsafe.Pointer(pCrlContext)))
	if r1 == 0 {
		return err
	}
	return nil
}

// CertGetCRLContextProperty gets an extended property for the specified certificate revocation list (CRL) context
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certgetcrlcontextproperty
func CertGetCRLContextProperty(pCrlContext *CRLContext, dwPropID uint32, pvData *byte, pcbData *uint32) error {

	r0, _, err := procCertGetCRLContextProperty.Call(
		uintptr(unsafe.Pointer(pCrlContext)),
		uintptr(dwPropID),
		uintptr(unsafe.Pointer(pvData)),
		uintptr(unsafe.Pointer(pcbData)),
	)
	if r0 == 0 {
		return err
	}
	return nil
}

// CertGetCertificateContextProperty retrieves the information contained in an extended property of a certificate context.
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certgetcertificatecontextproperty
func CertGetCertificateContextProperty(pCertContext *windows.CertContext, dwPropID uint32, pvData *byte, pcbData *uint32) error {
	r0, _, err := procCertGetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(pCertContext)),
		uintptr(dwPropID),
		uintptr(unsafe.Pointer(pvData)),
		uintptr(unsafe.Pointer(pcbData)),
	)
	if r0 == 0 {
		return err
	}
	return nil
}

// CertNameToStrW converts an encoded name in a windows.CertNameBlob structure to a null-terminated character string.
//
// https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certnametostrw
func CertNameToStrW(dwCertEncodingType uint32, pName *windows.CertNameBlob, dwStrType uint32, psz []uint16, csz uint32) (string, uint32, error) {
	var bufferPointer uintptr

	if psz == nil {
		bufferPointer = 0
	} else {
		bufferPointer = uintptr(unsafe.Pointer(&psz[0]))
	}

	r0, _, err := procCertNameToStr.Call(
		uintptr(dwCertEncodingType),
		uintptr(unsafe.Pointer(pName)),
		uintptr(dwStrType),
		bufferPointer,
		uintptr(csz),
	)

	if psz == nil || csz == 0 {
		bufferSize := uint32(r0)
		if bufferSize == 0 {
			return "", 0, err
		}
		return "", bufferSize, nil
	}

	if r0 == 0 {
		return "", 0, err
	}

	return windows.UTF16ToString(psz), 0, nil
}
