// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package winutil

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// LSA (Local Security Authority) helpers to retrieve secrets from the local machine.

var (
	modAdvapi32                = windows.NewLazySystemDLL("advapi32.dll")
	procLsaOpenPolicy          = modAdvapi32.NewProc("LsaOpenPolicy")
	procLsaRetrievePrivateData = modAdvapi32.NewProc("LsaRetrievePrivateData")
	procLsaClose               = modAdvapi32.NewProc("LsaClose")
	procLsaFreeMemory          = modAdvapi32.NewProc("LsaFreeMemory")
	procLsaNtStatusToWinError  = modAdvapi32.NewProc("LsaNtStatusToWinError")
)

// https://learn.microsoft.com/en-us/windows/win32/api/ntsecapi/ns-ntsecapi-lsa_unicode_string
type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

// https://learn.microsoft.com/en-us/windows/win32/api/ntsecapi/ns-ntsecapi-lsa_object_attributes
type lsaObjectAttributes struct {
	Length                   uint32
	RootDirectory            windows.Handle
	ObjectName               *lsaUnicodeString
	Attributes               uint32
	SecurityDescriptor       *byte
	SecurityQualityOfService *byte
}

const (
	policyGetPrivateInformation uint32 = 0x00000040
)

func initLsaUnicodeString(s string) lsaUnicodeString {
	if s == "" {
		return lsaUnicodeString{}
	}
	// UTF16FromString returns a slice including the terminating NUL
	u16, _ := windows.UTF16FromString(s)
	// Exclude terminating NUL when computing length; lengths are in bytes
	byteLen := (len(u16) - 1) * 2
	return lsaUnicodeString{Length: uint16(byteLen), MaximumLength: uint16(byteLen), Buffer: &u16[0]}
}

// getWinError converts NTSTATUS to a Win32 error code
func getWinError(status uintptr) error {
	if status == 0 {
		return nil
	}
	r0, _, _ := procLsaNtStatusToWinError.Call(status)
	return windows.Errno(r0)
}

// GetLSASecretString retrieves a secret value from LSA private data by key name.
func GetLSASecretString(key string) (string, error) {
	// Open local policy with POLICY_GET_PRIVATE_INFORMATION
	var policyHandle windows.Handle
	systemName := lsaUnicodeString{}
	objAttrs := lsaObjectAttributes{Length: uint32(unsafe.Sizeof(lsaObjectAttributes{}))}

	status, _, _ := procLsaOpenPolicy.Call(
		uintptr(unsafe.Pointer(&systemName)),
		uintptr(unsafe.Pointer(&objAttrs)),
		uintptr(policyGetPrivateInformation),
		uintptr(unsafe.Pointer(&policyHandle)),
	)
	if err := getWinError(status); err != nil {
		return "", fmt.Errorf("LsaOpenPolicy failed: %w", err)
	}
	defer procLsaClose.Call(uintptr(policyHandle))

	keyName := initLsaUnicodeString(key)
	var secretPtr *lsaUnicodeString
	status, _, _ = procLsaRetrievePrivateData.Call(
		uintptr(policyHandle),
		uintptr(unsafe.Pointer(&keyName)),
		uintptr(unsafe.Pointer(&secretPtr)),
	)
	if err := getWinError(status); err != nil {
		return "", fmt.Errorf("LsaRetrievePrivateData failed: %w", err)
	}
	if secretPtr == nil {
		return "", fmt.Errorf("LSA secret pointer is NULL")
	}
	defer procLsaFreeMemory.Call(uintptr(unsafe.Pointer(secretPtr)))

	secret := secretPtr
	if secret.Buffer == nil || secret.Length == 0 {
		return "", nil
	}
	// secret.Length is bytes; convert to uint16 length
	u16len := int(secret.Length / 2)
	buf := unsafe.Slice(secret.Buffer, u16len)
	return windows.UTF16ToString(buf), nil
}

// GetAgentUserPasswordFromLSA returns the Agent user password stored by the MSI in LSA.
func GetAgentUserPasswordFromLSA() (string, error) {
	// Matches the installer secret name
	const key = "L$datadog_ddagentuser_password"
	return GetLSASecretString(key)
}
