// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"encoding/base64"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// PDU represents a data unit from an SNMP request, with smart typing.
type PDU struct {
	OID  string         `json:"oid"`
	Type gosnmp.Asn1BER `json:"type"`
	// Value is the stringified version of the value; if Type is OctetString or BitString, it is base64 encoded.
	Value string `json:"value"`
}

// PDUFromSNMP packages a gosnmp.SnmpPDU as a PDU
func PDUFromSNMP(pdu *gosnmp.SnmpPDU) (*PDU, error) {
	value, err := GetValueFromPDU(*pdu)
	if err != nil {
		return nil, err
	}
	record := PDU{
		OID:  strings.TrimLeft(pdu.Name, "."),
		Type: pdu.Type,
	}
	if err := record.SetValue(value); err != nil {
		return nil, fmt.Errorf("unsupported PDU type for OID %s: %w", pdu.Name, err)
	}
	return &record, nil
}

// RawValue returns the value as a []byte, int64, float64, or plain string, depending on the type.
func (d *PDU) RawValue() (any, error) {
	switch d.Type {
	case gosnmp.OctetString, gosnmp.BitString:
		return base64.StdEncoding.DecodeString(d.Value)
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64, gosnmp.Uinteger32:
		return strconv.ParseInt(d.Value, 10, 64)
	case gosnmp.OpaqueFloat:
		return strconv.ParseFloat(d.Value, 32)
	case gosnmp.OpaqueDouble:
		return strconv.ParseFloat(d.Value, 64)
	case gosnmp.IPAddress, gosnmp.ObjectIdentifier:
		return d.Value, nil
	default:
		return nil, fmt.Errorf("oid %s: invalid type: %s", d.OID, d.Type.String())
	}
}

// SetValue sets the value. The input should be a []byte, int64, float64, or plain string, depending on the type.
func (d *PDU) SetValue(value any) error {
	var ok bool
	switch value := value.(type) {
	case []byte:
		if d.Type == gosnmp.OctetString || d.Type == gosnmp.BitString {
			d.Value = base64.StdEncoding.EncodeToString(value)
			ok = true
		}
	case int, int64:
		if slices.Contains([]gosnmp.Asn1BER{gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64, gosnmp.Uinteger32, gosnmp.OpaqueFloat, gosnmp.OpaqueDouble}, d.Type) {
			d.Value = fmt.Sprintf("%d", value)
			ok = true
		}
	case float32, float64:
		// If the ASN type is an integer type, cast the float to an int64 if (and only if) it's already integral.
		if slices.Contains([]gosnmp.Asn1BER{gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64, gosnmp.Uinteger32}, d.Type) {
			var f float64
			if v, is32 := value.(float32); is32 {
				f = float64(v)
			} else {
				f = value.(float64)
			}
			r := math.Round(f)
			if math.Abs(r-f) > 1e-6 {
				return fmt.Errorf("cannot use non-integer float %f as value for type %s", value, d.Type.String())
			}
			d.Value = fmt.Sprintf("%.0f", r)
			ok = true
		} else if slices.Contains([]gosnmp.Asn1BER{gosnmp.OpaqueFloat, gosnmp.OpaqueDouble}, d.Type) {
			d.Value = fmt.Sprintf("%f", value)
			ok = true
		}
	case string:
		if d.Type == gosnmp.IPAddress || d.Type == gosnmp.ObjectIdentifier {
			d.Value = value
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("cannot use %T as value for type %s", value, d.Type.String())
	}
	return nil
}
