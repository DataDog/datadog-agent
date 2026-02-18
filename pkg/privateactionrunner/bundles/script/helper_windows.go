// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows && !local

package com_datadoghq_script

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ScriptUserName is the low-privilege local account under which all
// PowerShell scripts are executed (mirrors Linux dd-scriptuser).
var ScriptUserName = "dd-scriptuser"

const (
	passwordLSAKey             = "L$datadog_scriptuser_password" // LSA key for the script user password
	logon32LogonBatch          = 4                               // LOGON32_LOGON_BATCH
	logon32ProviderDefault     = 0                               // LOGON32_PROVIDER_DEFAULT
	policyGetPrivateInformation = 0x00000004                     // LSA read-private-data access mask
)

var (
	modadvapi32                = windows.NewLazySystemDLL("advapi32.dll")
	procLogonUserW             = modadvapi32.NewProc("LogonUserW")
	procLsaOpenPolicy          = modadvapi32.NewProc("LsaOpenPolicy")
	procLsaRetrievePrivateData = modadvapi32.NewProc("LsaRetrievePrivateData")
	procLsaClose               = modadvapi32.NewProc("LsaClose")
	procLsaFreeMemory          = modadvapi32.NewProc("LsaFreeMemory")
)

type lsaUnicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type lsaObjectAttributes struct {
	Length                   uint32
	RootDirectory            uintptr
	ObjectName               uintptr
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

// logonScriptUser obtains a user token for dd-scriptuser by reading the
// password from LSA. The caller must close the returned token.
func logonScriptUser() (syscall.Token, error) {
	password, err := retrieveScriptUserPassword()
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve script user password from LSA: %w", err)
	}

	token, err := logonUser(ScriptUserName, ".", password, logon32LogonBatch, logon32ProviderDefault)
	if err != nil {
		return 0, fmt.Errorf("LogonUser failed for %s: %w", ScriptUserName, err)
	}

	return token, nil
}

// logonUser wraps LogonUserW to authenticate a local user and return a token.
func logonUser(username, domain, password string, logonType, logonProvider uint32) (syscall.Token, error) {
	usernameP, err := windows.UTF16PtrFromString(username)
	if err != nil {
		return 0, err
	}
	domainP, err := windows.UTF16PtrFromString(domain)
	if err != nil {
		return 0, err
	}
	passwordP, err := windows.UTF16PtrFromString(password)
	if err != nil {
		return 0, err
	}

	var token syscall.Token
	r1, _, lastErr := procLogonUserW.Call(
		uintptr(unsafe.Pointer(usernameP)),
		uintptr(unsafe.Pointer(domainP)),
		uintptr(unsafe.Pointer(passwordP)),
		uintptr(logonType),
		uintptr(logonProvider),
		uintptr(unsafe.Pointer(&token)),
	)
	if r1 == 0 {
		return 0, lastErr
	}
	return token, nil
}

// retrieveScriptUserPassword reads the script user password from LSA private data.
func retrieveScriptUserPassword() (string, error) {
	var policyHandle uintptr
	var objAttrs lsaObjectAttributes
	objAttrs.Length = uint32(unsafe.Sizeof(objAttrs))

	status, _, _ := procLsaOpenPolicy.Call(
		0,
		uintptr(unsafe.Pointer(&objAttrs)),
		policyGetPrivateInformation,
		uintptr(unsafe.Pointer(&policyHandle)),
	)
	if status != 0 {
		return "", fmt.Errorf("LsaOpenPolicy failed with NTSTATUS: 0x%08X", status)
	}
	defer procLsaClose.Call(policyHandle) //nolint:errcheck

	keyUTF16, err := windows.UTF16FromString(passwordLSAKey)
	if err != nil {
		return "", fmt.Errorf("failed to convert LSA key to UTF-16: %w", err)
	}
	keyStr := lsaUnicodeString{
		Length:        uint16((len(keyUTF16) - 1) * 2),
		MaximumLength: uint16(len(keyUTF16) * 2),
		Buffer:        &keyUTF16[0],
	}

	var privateData *lsaUnicodeString
	status, _, _ = procLsaRetrievePrivateData.Call(
		policyHandle,
		uintptr(unsafe.Pointer(&keyStr)),
		uintptr(unsafe.Pointer(&privateData)),
	)
	if status != 0 {
		return "", fmt.Errorf("LsaRetrievePrivateData failed with NTSTATUS: 0x%08X — ensure %s account was provisioned during installation", status, ScriptUserName)
	}
	if privateData != nil {
		defer procLsaFreeMemory.Call(uintptr(unsafe.Pointer(privateData))) //nolint:errcheck
	}

	if privateData == nil || privateData.Buffer == nil || privateData.Length == 0 {
		return "", fmt.Errorf("script user password not found in LSA (key: %s) — run the provisioning script to create the %s account", passwordLSAKey, ScriptUserName)
	}

	charCount := int(privateData.Length / 2)
	buf := unsafe.Slice(privateData.Buffer, charCount)
	password := windows.UTF16ToString(buf)
	return password, nil
}

// requiredWindowsEnvVars must always be passed for PowerShell to function.
var requiredWindowsEnvVars = []string{
	"SYSTEMROOT",
	"COMSPEC",
	"PATHEXT",
	"WINDIR",
	"TEMP",
	"TMP",
}

// buildAllowedEnv returns only the required + explicitly allowed env vars.
func buildAllowedEnv(envVarNames []string) []string {
	allowed := make(map[string]bool)
	for _, name := range requiredWindowsEnvVars {
		allowed[strings.ToUpper(name)] = true
	}
	for _, name := range envVarNames {
		allowed[strings.ToUpper(name)] = true
	}

	var env []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.ToUpper(parts[0])
		if allowed[name] {
			env = append(env, e)
		}
	}

	return env
}

// NewShellScriptCommand is not supported on Windows.
func NewShellScriptCommand(_ context.Context, _ string, _ []string) (*exec.Cmd, error) {
	return nil, errors.New("shell script execution is not supported on Windows; use runPredefinedPowershellScript")
}

// NewPredefinedScriptCommand is not supported on Windows.
func NewPredefinedScriptCommand(_ context.Context, _ []string, _ []string) (*exec.Cmd, error) {
	return nil, errors.New("predefined script execution via shell is not supported on Windows; use runPredefinedPowershellScript")
}

