package gosnmplib

import (
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

func Test_getValueFromPDU(t *testing.T) {
	tests := []struct {
		caseName          string
		pduVariable       gosnmp.SnmpPDU
		expectedName      string
		expectedSnmpValue valuestore.ResultValue
		expectedErr       error
	}{
		{
			"Name",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(141)},
			nil,
		},
		{
			"Integer",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(141)},
			nil,
		},
		{
			"OctetString",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OctetString,
				Value: []byte(`myVal`),
			},
			"1.2.3",
			valuestore.ResultValue{Value: "myVal"},
			nil,
		},
		{
			"OctetString hexify",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OctetString,
				Value: []uint8{0x0, 0x24, 0x9b, 0x35, 0x3, 0xf6},
			},
			"1.2.3",
			valuestore.ResultValue{Value: "0x00249b3503f6"},
			nil,
		},
		{
			"BitString",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.BitString,
				Value: []byte(`myVal`),
			},
			"1.2.3",
			valuestore.ResultValue{Value: "myVal"},
			nil,
		},
		{
			"ObjectIdentifier",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.2",
			},
			"1.2.3",
			valuestore.ResultValue{Value: "1.2.2"},
			nil,
		},
		{
			"ObjectIdentifier need trim",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.ObjectIdentifier,
				Value: ".1.2.2",
			},
			"1.2.3",
			valuestore.ResultValue{Value: "1.2.2"},
			nil,
		},
		{
			"IPAddress",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.IPAddress,
				Value: "1.2.3.4",
			},
			"1.2.3",
			valuestore.ResultValue{Value: "1.2.3.4"},
			nil,
		},
		{
			"IPAddress invalid value",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.IPAddress,
				Value: nil,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: IPAddress should be string type but got <nil> type: gosnmp.SnmpPDU{Name:\".1.2.3\", Type:0x40, Value:interface {}(nil)}"),
		},
		{
			"Null",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Null,
				Value: nil,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: invalid type: Null"),
		},
		{
			"Counter32",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Counter32,
				Value: uint(10),
			},
			"1.2.3",
			valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			nil,
		},
		{
			"Gauge32",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Gauge32,
				Value: uint(10),
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(10)},
			nil,
		},
		{
			"TimeTicks",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.TimeTicks,
				Value: uint32(10),
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(10)},
			nil,
		},
		{
			"Counter64",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Counter64,
				Value: uint64(10),
			},
			"1.2.3",
			valuestore.ResultValue{SubmissionType: "counter", Value: float64(10)},
			nil,
		},
		{
			"Uinteger32",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.Uinteger32,
				Value: uint32(10),
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(10)},
			nil,
		},
		{
			"OpaqueFloat",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OpaqueFloat,
				Value: float32(10),
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(10)},
			nil,
		},
		{
			"OpaqueDouble",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OpaqueDouble,
				Value: float64(10),
			},
			"1.2.3",
			valuestore.ResultValue{Value: float64(10)},
			nil,
		},
		{
			"NoSuchObject",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.NoSuchObject,
				Value: nil,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: invalid type: NoSuchObject"),
		},
		{
			"NoSuchInstance",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.NoSuchInstance,
				Value: nil,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: invalid type: NoSuchInstance"),
		},
		{
			"gosnmp.OctetString with wrong type",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OctetString,
				Value: 1.0,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: OctetString/BitString should be []byte type but got float64 type: gosnmp.SnmpPDU{Name:\".1.2.3\", Type:0x4, Value:1}"),
		},
		{
			"gosnmp.OpaqueFloat with wrong type",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OpaqueFloat,
				Value: "abc",
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: OpaqueFloat should be float32 type but got string type: gosnmp.SnmpPDU{Name:\".1.2.3\", Type:0x78, Value:\"abc\"}"),
		},
		{
			"gosnmp.OpaqueDouble with wrong type",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.OpaqueDouble,
				Value: "abc",
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: OpaqueDouble should be float64 type but got string type: gosnmp.SnmpPDU{Name:\".1.2.3\", Type:0x79, Value:\"abc\"}"),
		},
		{
			"gosnmp.ObjectIdentifier with wrong type",
			gosnmp.SnmpPDU{
				Name:  ".1.2.3",
				Type:  gosnmp.ObjectIdentifier,
				Value: 1,
			},
			"1.2.3",
			valuestore.ResultValue{},
			fmt.Errorf("oid .1.2.3: ObjectIdentifier should be string type but got int type: gosnmp.SnmpPDU{Name:\".1.2.3\", Type:0x6, Value:1}"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			name, value, err := GetValueFromPDU(tt.pduVariable)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedSnmpValue, value)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}

func Test_resultToColumnValues(t *testing.T) {
	tests := []struct {
		name                string
		columnOids          []string
		snmpPacket          *gosnmp.SnmpPacket
		expectedValues      valuestore.ColumnResultValuesType
		expectedNextOidsMap map[string]string
	}{
		{
			"simple nominal case",
			[]string{"1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.2", "1.3.6.1.2.1.2.2.1.20"},
			&gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.1",
						Type:  gosnmp.Integer,
						Value: 141,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.2.1",
						Type:  gosnmp.OctetString,
						Value: []byte("desc1"),
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.20.1",
						Type:  gosnmp.Integer,
						Value: 201,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.2",
						Type:  gosnmp.Integer,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.2.2",
						Type:  gosnmp.OctetString,
						Value: []byte("desc2"),
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.20.2",
						Type:  gosnmp.Integer,
						Value: 202,
					},
				},
			},
			valuestore.ColumnResultValuesType{
				"1.3.6.1.2.1.2.2.1.14": {
					"1": valuestore.ResultValue{
						Value: float64(141),
					},
					"2": valuestore.ResultValue{
						Value: float64(142),
					},
				},
				"1.3.6.1.2.1.2.2.1.2": {
					"1": valuestore.ResultValue{
						Value: "desc1",
					},
					"2": valuestore.ResultValue{
						Value: "desc2",
					},
				},
				"1.3.6.1.2.1.2.2.1.20": {
					"1": valuestore.ResultValue{
						Value: float64(201),
					},
					"2": valuestore.ResultValue{
						Value: float64(202),
					},
				},
			},
			map[string]string{
				"1.3.6.1.2.1.2.2.1.14": "1.3.6.1.2.1.2.2.1.14.2",
				"1.3.6.1.2.1.2.2.1.2":  "1.3.6.1.2.1.2.2.1.2.2",
				"1.3.6.1.2.1.2.2.1.20": "1.3.6.1.2.1.2.2.1.20.2",
			},
		},
		{
			"no such object is skipped",
			[]string{"1.3.6.1.2.1.2.2.1.14", "1.3.6.1.2.1.2.2.1.2"},
			&gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name: "1.3.6.1.2.1.2.2.1.14.1",
						Type: gosnmp.NoSuchObject,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.2.1",
						Type:  gosnmp.OctetString,
						Value: []byte("desc1"),
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.2",
						Type:  gosnmp.Integer,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.2.2",
						Type:  gosnmp.OctetString,
						Value: []byte("desc2"),
					},
				},
			},
			valuestore.ColumnResultValuesType{
				"1.3.6.1.2.1.2.2.1.14": {
					// index 1 not fetched because of gosnmp.NoSuchObject error
					"2": valuestore.ResultValue{
						Value: float64(142),
					},
				},
				"1.3.6.1.2.1.2.2.1.2": {
					"1": valuestore.ResultValue{
						Value: "desc1",
					},
					"2": valuestore.ResultValue{
						Value: "desc2",
					},
				},
			},
			map[string]string{
				"1.3.6.1.2.1.2.2.1.14": "1.3.6.1.2.1.2.2.1.14.2",
				"1.3.6.1.2.1.2.2.1.2":  "1.3.6.1.2.1.2.2.1.2.2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, nextOidsMap := ResultToColumnValues(tt.columnOids, tt.snmpPacket)
			assert.Equal(t, tt.expectedValues, values)
			assert.Equal(t, tt.expectedNextOidsMap, nextOidsMap)
		})
	}
}

func Test_resultToScalarValues(t *testing.T) {
	tests := []struct {
		name           string
		snmpPacket     *gosnmp.SnmpPacket
		expectedValues valuestore.ScalarResultValuesType
	}{
		{
			"simple case",
			&gosnmp.SnmpPacket{
				Variables: []gosnmp.SnmpPDU{
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.1",
						Type:  gosnmp.Integer,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.2",
						Type:  gosnmp.Counter32,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.3",
						Type:  gosnmp.NoSuchInstance,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.4",
						Type:  gosnmp.NoSuchObject,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.5",
						Type:  gosnmp.EndOfContents,
						Value: 142,
					},
					{
						Name:  "1.3.6.1.2.1.2.2.1.14.6",
						Type:  gosnmp.EndOfMibView,
						Value: 142,
					},
				},
			},
			valuestore.ScalarResultValuesType{
				"1.3.6.1.2.1.2.2.1.14.1": {
					Value: float64(142),
				},
				"1.3.6.1.2.1.2.2.1.14.2": {
					SubmissionType: "counter",
					Value:          float64(142),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := ResultToScalarValues(tt.snmpPacket)
			assert.Equal(t, tt.expectedValues, values)
		})
	}
}
