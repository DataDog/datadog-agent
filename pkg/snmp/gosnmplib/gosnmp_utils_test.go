// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestPacketToString(t *testing.T) {
	tests := []struct {
		name        string
		packet      *gosnmp.SnmpPacket
		expectedStr string
	}{
		{
			name: "to string",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.ObjectIdentifier,
						Value: "1.3.6.1.4.1.3375.2.1.3.4.1",
					},
					{
						Name:  "1.3.6.1.2.1.1.3.0",
						Type:  gosnmp.Counter32,
						Value: 10,
					},
				},
			},
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"ObjectIdentifier\",\"value\":\"1.3.6.1.4.1.3375.2.1.3.4.1\"},{\"oid\":\"1.3.6.1.2.1.1.3.0\",\"type\":\"Counter32\",\"value\":\"10\"}]",
		},
		{
			name: "invalid ipaddr",
			packet: &gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.1.2.0",
						Type:  gosnmp.IPAddress,
						Value: 10,
					},
				},
			},
			expectedStr: "error=NoError(code:0, idx:0), values=[{\"oid\":\"1.3.6.1.2.1.1.2.0\",\"type\":\"IPAddress\",\"value\":\"10\",\"parse_err\":\"`oid 1.3.6.1.2.1.1.2.0: IPAddress should be string type but got type `int` and value `10``\"}]",
		},
		{
			name:        "nil packet loglevel",
			expectedStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := PacketAsString(tt.packet)
			assert.Equal(t, tt.expectedStr, str)
		})
	}
}

func TestIsStringPrintable(t *testing.T) {
	tests := []struct {
		name     string
		value    []byte
		expected bool
	}{
		{
			name:     "string",
			value:    []byte("test"),
			expected: true,
		},
		{
			name:     "with trailing null",
			value:    append([]byte("test"), 00),
			expected: true,
		},
		{
			name:     "not printable",
			value:    []byte{01, 02, 03},
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsStringPrintable(tt.value))
		})
	}
}

func TestGetValueFromPDU(t *testing.T) {
	tests := []struct {
		name      string
		pdu       gosnmp.SnmpPDU
		expected  interface{}
		expectErr bool
	}{
		{
			name:     "OctetString",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OctetString, Value: []byte("hello")},
			expected: []byte("hello"),
		},
		{
			name:      "OctetString wrong type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OctetString, Value: "not bytes"},
			expectErr: true,
		},
		{
			name:     "BitString",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.BitString, Value: []byte{0x01, 0x02}},
			expected: []byte{0x01, 0x02},
		},
		{
			name:     "Integer",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.Integer, Value: 42},
			expected: float64(42),
		},
		{
			name:     "Counter32",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.Counter32, Value: uint(100)},
			expected: float64(100),
		},
		{
			name:     "Counter64",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.Counter64, Value: uint64(999)},
			expected: float64(999),
		},
		{
			name:     "Gauge32",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.Gauge32, Value: uint(50)},
			expected: float64(50),
		},
		{
			name:     "TimeTicks",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.TimeTicks, Value: uint32(12345)},
			expected: float64(12345),
		},
		{
			name:     "OpaqueFloat valid",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OpaqueFloat, Value: float32(3.14)},
			expected: float64(float32(3.14)),
		},
		{
			name:      "OpaqueFloat wrong type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OpaqueFloat, Value: "not float"},
			expectErr: true,
		},
		{
			name:     "OpaqueDouble valid",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OpaqueDouble, Value: float64(2.71828)},
			expected: float64(2.71828),
		},
		{
			name:      "OpaqueDouble wrong type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.OpaqueDouble, Value: "not double"},
			expectErr: true,
		},
		{
			name:     "IPAddress valid",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.IPAddress, Value: "192.168.1.1"},
			expected: "192.168.1.1",
		},
		{
			name:      "IPAddress wrong type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.IPAddress, Value: 12345},
			expectErr: true,
		},
		{
			name:     "ObjectIdentifier strips leading dot",
			pdu:      gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4"},
			expected: "1.3.6.1.4",
		},
		{
			name:      "ObjectIdentifier wrong type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.ObjectIdentifier, Value: 12345},
			expectErr: true,
		},
		{
			name:      "unsupported type",
			pdu:       gosnmp.SnmpPDU{Name: "1.2.3", Type: gosnmp.NoSuchObject, Value: nil},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := GetValueFromPDU(tt.pdu)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, val)
			}
		})
	}
}

func TestConnectionError(t *testing.T) {
	origErr := assert.AnError
	connErr := NewConnectionError(origErr)

	assert.Equal(t, origErr.Error(), connErr.Error())
	assert.Equal(t, origErr, connErr.Unwrap())
}

func TestStandardTypeToString(t *testing.T) {
	tests := []struct {
		name        string
		value       interface{}
		expectedStr string
		expectErr   bool
	}{
		{
			name:        "float64",
			value:       float64(123),
			expectedStr: "123",
			expectErr:   false,
		},
		{
			name:        "string",
			value:       "test string",
			expectedStr: "test string",
			expectErr:   false,
		},
		{
			name:        "printable byte array",
			value:       []byte("hello world"),
			expectedStr: "hello world",
			expectErr:   false,
		},
		{
			name:        "non-printable byte array",
			value:       []byte{0x01, 0x02, 0x03},
			expectedStr: "0x010203",
			expectErr:   false,
		},
		{
			name:        "byte array with trailing nulls",
			value:       append([]byte("test"), 0x00, 0x00),
			expectedStr: "test",
			expectErr:   false,
		},
		{
			name:        "invalid type",
			value:       []int{1, 2, 3},
			expectedStr: "",
			expectErr:   true,
		},
		{
			name:        "zeros should be hexified",
			value:       []byte{0x00, 0x00, 0x00},
			expectedStr: "0x000000",
			expectErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str, err := StandardTypeToString(tt.value)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStr, str)
			}
		})
	}
}
