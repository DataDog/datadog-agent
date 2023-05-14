// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func Test_getAuthProtocol(t *testing.T) {
	tests := []struct {
		authProtocolStr      string
		expectedAuthProtocol gosnmp.SnmpV3AuthProtocol
		expectedError        error
	}{
		{
			"invalid",
			gosnmp.NoAuth,
			fmt.Errorf("unsupported authentication protocol: invalid"),
		},
		{
			"",
			gosnmp.NoAuth,
			nil,
		},
		{
			"md5",
			gosnmp.MD5,
			nil,
		},
		{
			"MD5",
			gosnmp.MD5,
			nil,
		},
		{
			"sha",
			gosnmp.SHA,
			nil,
		},
		{
			"sha224",
			gosnmp.SHA224,
			nil,
		},
		{
			"sha256",
			gosnmp.SHA256,
			nil,
		},
		{
			"sha384",
			gosnmp.SHA384,
			nil,
		},
		{
			"sha512",
			gosnmp.SHA512,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.authProtocolStr, func(t *testing.T) {
			authProtocol, err := GetAuthProtocol(tt.authProtocolStr)
			assert.Equal(t, tt.expectedError, err)
			assert.Equal(t, tt.expectedAuthProtocol, authProtocol)
		})
	}
}

func Test_getPrivProtocol(t *testing.T) {
	tests := []struct {
		privProtocolStr  string
		expectedProtocol gosnmp.SnmpV3PrivProtocol
		expectedError    error
	}{
		{
			"invalid",
			gosnmp.NoPriv,
			fmt.Errorf("unsupported privacy protocol: invalid"),
		},
		{
			"",
			gosnmp.NoPriv,
			nil,
		},
		{
			"des",
			gosnmp.DES,
			nil,
		},
		{
			"DES",
			gosnmp.DES,
			nil,
		},
		{
			"aes",
			gosnmp.AES,
			nil,
		},
		{
			"aes192",
			gosnmp.AES192,
			nil,
		},
		{
			"aes256",
			gosnmp.AES256,
			nil,
		},
		{
			"aes192c",
			gosnmp.AES192C,
			nil,
		},
		{
			"aes256c",
			gosnmp.AES256C,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.privProtocolStr, func(t *testing.T) {
			privProtocol, err := GetPrivProtocol(tt.privProtocolStr)
			assert.Equal(t, tt.expectedError, err)
			assert.Equal(t, tt.expectedProtocol, privProtocol)
		})
	}
}
