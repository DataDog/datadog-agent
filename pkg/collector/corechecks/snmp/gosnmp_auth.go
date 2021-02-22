package snmp

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

func getAuthProtocol(authProtocolStr string) (gosnmp.SnmpV3AuthProtocol, error) {
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

func getPrivProtocol(privProtocolStr string) (gosnmp.SnmpV3PrivProtocol, error) {
	var privProtocol gosnmp.SnmpV3PrivProtocol
	lowerPrivProtocol := strings.ToLower(privProtocolStr)
	if lowerPrivProtocol == "" {
		privProtocol = gosnmp.NoPriv
	} else if lowerPrivProtocol == "des" {
		privProtocol = gosnmp.DES
	} else if lowerPrivProtocol == "aes" {
		privProtocol = gosnmp.AES
	} else if lowerPrivProtocol == "aes192" {
		privProtocol = gosnmp.AES192
	} else if lowerPrivProtocol == "aes192c" {
		privProtocol = gosnmp.AES192C
	} else if lowerPrivProtocol == "aes256" {
		privProtocol = gosnmp.AES256
	} else if lowerPrivProtocol == "aes256c" {
		privProtocol = gosnmp.AES256C
	} else {
		return gosnmp.NoPriv, fmt.Errorf("unsupported privacy protocol: %s", privProtocolStr)
	}
	return privProtocol, nil
}
