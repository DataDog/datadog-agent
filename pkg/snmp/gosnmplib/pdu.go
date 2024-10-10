// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"strings"

	"github.com/gosnmp/gosnmp"
)

// PDU represents a data unit from an SNMP request, with smart typing.
type PDU struct {
	OID  string `json:"oid"`
	Type string `json:"type"`
	// Value is the stringified version of the value; if Type is OctetString or BitString, it is base64 encoded.
	Value string `json:"value_as_string"`
}

// PDUFromSNMP packages a gosnmp.SnmpPDU as a PDU
func PDUFromSNMP(pdu *gosnmp.SnmpPDU) (*PDU, error) {
	value, err := GetStringValueFromPDU(*pdu)
	if err != nil {
		return nil, err
	}
	record := PDU{
		OID:   strings.TrimLeft(pdu.Name, "."),
		Type:  pdu.Type.String(),
		Value: value,
	}
	return &record, nil
}
