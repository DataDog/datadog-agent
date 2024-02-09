// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package gosnmplib

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// GetAuthProtocol converts auth protocol from string to type
func GetAuthProtocol(authProtocolStr string) (gosnmp.SnmpV3AuthProtocol, error) {
	var authProtocol gosnmp.SnmpV3AuthProtocol
	lowerAuthProtocol := strings.ToLower(authProtocolStr)
	if lowerAuthProtocol == "" {
		authProtocol = gosnmp.NoAuth
	} else if lowerAuthProtocol == "md5" {
		authProtocol = gosnmp.MD5
	} else if lowerAuthProtocol == "sha" {
		authProtocol = gosnmp.SHA
	} else if lowerAuthProtocol == "sha224" {
		authProtocol = gosnmp.SHA224
	} else if lowerAuthProtocol == "sha256" {
		authProtocol = gosnmp.SHA256
	} else if lowerAuthProtocol == "sha384" {
		authProtocol = gosnmp.SHA384
	} else if lowerAuthProtocol == "sha512" {
		authProtocol = gosnmp.SHA512
	} else {
		return gosnmp.NoAuth, fmt.Errorf("unsupported authentication protocol: %s", authProtocolStr)
	}
	return authProtocol, nil
}

// GetPrivProtocol converts priv protocol from string to type
// Related resource: https://github.com/gosnmp/gosnmp/blob/f6fb3f74afc3fb0e5b44b3f60751b988bc960019/v3_usm.go#L458-L461
// Reeder AES192/256: Used by many vendors, including Cisco.
// Blumenthal AES192/256: Not many vendors use this algorithm.
func GetPrivProtocol(privProtocolStr string) (gosnmp.SnmpV3PrivProtocol, error) {
	var privProtocol gosnmp.SnmpV3PrivProtocol
	lowerPrivProtocol := strings.ToLower(privProtocolStr)
	if lowerPrivProtocol == "" {
		privProtocol = gosnmp.NoPriv
	} else if lowerPrivProtocol == "des" {
		privProtocol = gosnmp.DES
	} else if lowerPrivProtocol == "aes" {
		privProtocol = gosnmp.AES
	} else if lowerPrivProtocol == "aes192" {
		privProtocol = gosnmp.AES192 // Blumenthal-AES192
	} else if lowerPrivProtocol == "aes256" {
		privProtocol = gosnmp.AES256 // Blumenthal-AES256
	} else if lowerPrivProtocol == "aes192c" {
		privProtocol = gosnmp.AES192C // Reeder-AES192
	} else if lowerPrivProtocol == "aes256c" {
		privProtocol = gosnmp.AES256C // Reeder-AES256
	} else {
		return gosnmp.NoPriv, fmt.Errorf("unsupported privacy protocol: %s", privProtocolStr)
	}
	return privProtocol, nil
}
