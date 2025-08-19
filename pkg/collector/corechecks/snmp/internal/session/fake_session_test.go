// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package session

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func BytePDU(oid string, value []byte) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{
		Name:  oid,
		Type:  gosnmp.OctetString,
		Value: value,
	}
}

func StrPDU(oid string, value string) gosnmp.SnmpPDU {
	return BytePDU(oid, []byte(value))
}

func ObjPDU(oid string, value string) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{
		Name:  oid,
		Type:  gosnmp.ObjectIdentifier,
		Value: value,
	}
}

func NoObjPDU(oid string) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{
		Name:  oid,
		Type:  gosnmp.NoSuchObject,
		Value: nil,
	}
}

func EndOfMibViewPDU(oid string) gosnmp.SnmpPDU {
	return gosnmp.SnmpPDU{
		Name:  oid,
		Type:  gosnmp.EndOfMibView,
		Value: nil,
	}
}

func Packet(pdus ...gosnmp.SnmpPDU) *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{
		Variables: pdus,
	}
}

func TestFakeSession(t *testing.T) {
	sess := CreateFakeSession()

	// system description
	sess.SetStr("1.3.6.1.2.1.1.1.0", "my_desc")
	// device type
	sess.SetObj("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1")
	// interface description
	sess.SetStr("1.3.6.1.2.1.2.2.1.2.1", `desc1`)
	// interface physical address
	sess.SetByte("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01})
	// interface description
	sess.SetStr("1.3.6.1.2.1.2.2.1.2.2", `desc2`)
	// interface physical address
	sess.SetByte("1.3.6.1.2.1.2.2.1.6.2", []byte{00, 00, 00, 00, 00, 01})

	t.Run("Get", func(t *testing.T) {
		tests := []struct {
			name     string
			oids     []string
			expected *gosnmp.SnmpPacket
		}{{
			name: "single",
			oids: []string{"1.3.6.1.2.1.1.2.0"},
			expected: Packet(
				ObjPDU("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1"),
			),
		}, {
			name: "multiple",
			oids: []string{"1.3.6.1.2.1.1.2.0", "1.3.6.1.2.1.1.1.0"},
			expected: Packet(
				ObjPDU("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1"),
				StrPDU("1.3.6.1.2.1.1.1.0", "my_desc"),
			),
		}, {
			name: "missing",
			oids: []string{"1.3.6.1.2.1.1.2.0", "1.3.6.1.2.1.1.10"},
			expected: Packet(
				ObjPDU("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1"),
				NoObjPDU("1.3.6.1.2.1.1.10"),
			),
		}, {
			name: "invalid",
			oids: []string{"1.3.6.1.2.1.1.2.0", "not.an.oid"},
			expected: Packet(
				ObjPDU("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.3375.2.1.3.4.1"),
				NoObjPDU("not.an.oid"),
			),
		}}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := sess.Get(tt.oids)
				assert.Nil(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("GetNext", func(t *testing.T) {
		tests := []struct {
			name     string
			oids     []string
			expected *gosnmp.SnmpPacket
		}{{
			name:     "single",
			oids:     []string{"1.0"},
			expected: Packet(StrPDU("1.3.6.1.2.1.1.1.0", "my_desc")),
		}, {
			name: "multiple",
			oids: []string{"1.3.6.1.2.1.2.2.1.2.0", "1.3.6.1.2.1.2.2.1.6.0"},
			expected: Packet(
				StrPDU("1.3.6.1.2.1.2.2.1.2.1", `desc1`), BytePDU("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}),
			),
		}, {
			name: "no-next",
			oids: []string{"1.3.6.1.2.1.2.2.1.6.2"},
			expected: Packet(
				EndOfMibViewPDU("1.3.6.1.2.1.2.2.1.6.2"),
			),
		}, {
			name: "invalid",
			oids: []string{"not.an.oid"},
			expected: Packet(
				EndOfMibViewPDU("not.an.oid"),
			),
		}}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := sess.GetNext(tt.oids)
				assert.Nil(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("GetBulk", func(t *testing.T) {
		tests := []struct {
			name     string
			oids     []string
			count    uint32
			expected *gosnmp.SnmpPacket
		}{{
			name:  "single",
			oids:  []string{"1.3.6.1.2.1.2.2.1.2.0"},
			count: 3,
			expected: Packet(
				StrPDU("1.3.6.1.2.1.2.2.1.2.1", `desc1`),
				StrPDU("1.3.6.1.2.1.2.2.1.2.2", `desc2`),
				BytePDU("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}),
			),
		}, {
			name:  "multiple",
			oids:  []string{"1.3.6.1.2.1.2.2.1.2.0", "1.3.6.1.2.1.2.2.1.6.0"},
			count: 3,
			expected: Packet(
				StrPDU("1.3.6.1.2.1.2.2.1.2.1", `desc1`),
				BytePDU("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}),
				StrPDU("1.3.6.1.2.1.2.2.1.2.2", `desc2`),
				BytePDU("1.3.6.1.2.1.2.2.1.6.2", []byte{00, 00, 00, 00, 00, 01}),
				BytePDU("1.3.6.1.2.1.2.2.1.6.1", []byte{00, 00, 00, 00, 00, 01}),
				EndOfMibViewPDU("1.3.6.1.2.1.2.2.1.6.0"),
			),
		}, {
			name:  "no-next",
			count: 3,
			oids:  []string{"1.3.6.1.2.1.2.2.1.6.2"},
			expected: Packet(
				EndOfMibViewPDU("1.3.6.1.2.1.2.2.1.6.2"),
				EndOfMibViewPDU("1.3.6.1.2.1.2.2.1.6.2"),
				EndOfMibViewPDU("1.3.6.1.2.1.2.2.1.6.2"),
			),
		}, {
			name:  "invalid",
			count: 3,
			oids:  []string{"not.an.oid"},
			expected: Packet(
				EndOfMibViewPDU("not.an.oid"),
				EndOfMibViewPDU("not.an.oid"),
				EndOfMibViewPDU("not.an.oid"),
			),
		}}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := sess.GetBulk(tt.oids, tt.count)
				assert.Nil(t, err)
				assert.Equal(t, tt.expected, result)
			})
		}
	})
}
