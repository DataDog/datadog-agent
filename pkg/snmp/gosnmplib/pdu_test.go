// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

type testCase struct {
	asn1ber gosnmp.Asn1BER
	input   any
	value   string
	// nil if the input should cause an error
	rawValue any
}

func TestDeviceOID(t *testing.T) {
	for _, tc := range []testCase{
		{gosnmp.BitString, []byte("hello, world"), "aGVsbG8sIHdvcmxk", []byte("hello, world")},
		{gosnmp.OctetString, []byte("hello, world"), "aGVsbG8sIHdvcmxk", []byte("hello, world")},
		{gosnmp.OctetString, "hello, world", "", nil},
		{gosnmp.OctetString, 100, "", nil},
		{gosnmp.OctetString, "aGVsbG8sIHdvcmxk", "", nil},
		{gosnmp.Counter32, 100, "100", int64(100)},
		{gosnmp.Counter64, 100, "100", int64(100)},
		{gosnmp.Gauge32, 100, "100", int64(100)},
		{gosnmp.Uinteger32, 100, "100", int64(100)},
		{gosnmp.TimeTicks, 100, "100", int64(100)},
		{gosnmp.Integer, 100, "100", int64(100)},
		{gosnmp.Integer, 100.01, "", nil},
		{gosnmp.Integer, "hello, world", "", nil},
		{gosnmp.Integer, []byte("100"), "", nil},
		{gosnmp.Integer, "100", "", nil},
		{gosnmp.Integer, float32(101.00), "101", int64(101)},
		{gosnmp.Integer, float64(-95.00), "-95", int64(-95)},
		{gosnmp.OpaqueDouble, 100, "100", float64(100)},
		{gosnmp.OpaqueFloat, 100, "100", float64(100)},
		{gosnmp.OpaqueFloat, "hello, world", "", nil},
		{gosnmp.OpaqueFloat, []byte("100"), "", nil},
		{gosnmp.OpaqueFloat, "100", "", nil},
		{gosnmp.OpaqueFloat, 100.1, "100.100000", float64(100.1)},
		{gosnmp.OpaqueFloat, float64(100.1), "100.100000", float64(100.1)},
		{gosnmp.ObjectIdentifier, "1.2.3.4.5.6.7", "1.2.3.4.5.6.7", "1.2.3.4.5.6.7"},
		{gosnmp.IPAddress, "127.0.0.1", "127.0.0.1", "127.0.0.1"},
		{gosnmp.IPAddress, []byte("127.0.0.1"), "", nil},
		{gosnmp.IPAddress, 127, "", nil},
	} {
		d := gosnmplib.PDU{
			Type: tc.asn1ber,
		}
		if tc.rawValue == nil {
			assert.Error(t, d.SetValue(tc.input))
		} else {
			assert.NoError(t, d.SetValue(tc.input))
			assert.Equal(t, tc.value, d.Value)
			val, err := d.RawValue()
			assert.NoError(t, err)
			switch tval := val.(type) {
			case float32, float64:
				assert.InEpsilon(t, tc.rawValue, tval, 1e-6)
			default:
				assert.Equal(t, tc.rawValue, tval)
			}
		}
	}
	t.Run("BadValue", func(t *testing.T) {
		for _, tc := range []struct {
			asn1ber gosnmp.Asn1BER
			value   string
		}{
			{gosnmp.Counter32, "not_an_int"},
			{gosnmp.Counter32, "1.05"},
			{gosnmp.OpaqueDouble, "not_a_number"},
			{gosnmp.OctetString, "invalid_bytes"},
		} {
			d := gosnmplib.PDU{
				Type:  tc.asn1ber,
				Value: tc.value,
			}
			_, err := d.RawValue()
			assert.Error(t, err)
		}
	})
}
