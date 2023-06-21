// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// GetAuthProtocol converts auth protocol from string to type
func GetAuthProtocol(authProtocolStr string) (gosnmp.SnmpV3AuthProtocol, error) {
	var authProtocol gosnmp.SnmpV3AuthProtocol
	if !SwitchAuthProtocol(authProtocolStr, &authProtocol) {
		return gosnmp.NoAuth, fmt.Errorf("unsupported authentication protocol: %s", authProtocolStr)
	}
	return authProtocol, nil
}

func SwitchAuthProtocol(authProtocol string, finalAuthProtocol *gosnmp.SnmpV3AuthProtocol) bool {
	switch strings.ToLower(authProtocol) {
	case "":
		*finalAuthProtocol = gosnmp.NoAuth
	case "md5":
		*finalAuthProtocol = gosnmp.MD5
	case "sha":
		*finalAuthProtocol = gosnmp.SHA
	case "sha224", "sha-224":
		*finalAuthProtocol = gosnmp.SHA224
	case "sha256", "sha-256":
		*finalAuthProtocol = gosnmp.SHA256
	case "sha384", "sha-384":
		*finalAuthProtocol = gosnmp.SHA384
	case "sha512", "sha-512":
		*finalAuthProtocol = gosnmp.SHA512
	default:
		return false
	}
	return true
}

// GetPrivProtocol converts priv protocol from string to type
// Related resource: https://github.com/gosnmp/gosnmp/blob/f6fb3f74afc3fb0e5b44b3f60751b988bc960019/v3_usm.go#L458-L461
// Reeder AES192/256: Used by many vendors, including Cisco.
// Blumenthal AES192/256: Not many vendors use this algorithm.
func GetPrivProtocol(privProtocolStr string) (gosnmp.SnmpV3PrivProtocol, error) {
	var privProtocol gosnmp.SnmpV3PrivProtocol
	if !SwitchPrivProtocol(privProtocolStr, &privProtocol) {
		return gosnmp.NoPriv, fmt.Errorf("unsupported privacy protocol: %s", privProtocolStr)
	}
	return privProtocol, nil
}

func SwitchPrivProtocol(privProtocol string, finalPrivProtocol *gosnmp.SnmpV3PrivProtocol) bool {
	switch strings.ToLower(privProtocol) {
	case "":
		*finalPrivProtocol = gosnmp.NoPriv
	case "des":
		*finalPrivProtocol = gosnmp.DES
	case "aes":
		*finalPrivProtocol = gosnmp.AES
	case "aes192", "aes-192":
		*finalPrivProtocol = gosnmp.AES192
	case "aes192c", "aes-192c", "aes-192-c":
		*finalPrivProtocol = gosnmp.AES192C
	case "aes256", "aes-256":
		*finalPrivProtocol = gosnmp.AES256
	case "aes256c", "aes-256c", "aes-256-c":
		*finalPrivProtocol = gosnmp.AES256C
	default:
		return false
	}
	return true
}
