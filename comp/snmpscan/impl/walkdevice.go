// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

// RunSnmpWalk prints every SNMP value, in the style of the unix snmpwalk command.
func (s snmpScannerImpl) RunSnmpWalk(snmpConnection *gosnmp.GoSNMP, firstOid string) error {
	// Perform a snmpwalk using Walk for all versions
	if err := snmpConnection.Walk(firstOid, printValue); err != nil {
		return fmt.Errorf("unable to walk SNMP agent on %s:%d: %w", snmpConnection.Target, snmpConnection.Port, err)
	}

	return nil
}

// printValue prints a PDU in a similar style to snmpwalk -Ont
func printValue(pdu gosnmp.SnmpPDU) error {
	fmt.Printf("%s = ", pdu.Name)

	switch pdu.Type {
	case gosnmp.OctetString:
		b := pdu.Value.([]byte)
		if !gosnmplib.IsStringPrintable(b) {
			var strBytes []string
			for _, bt := range b {
				strBytes = append(strBytes, strings.ToUpper(hex.EncodeToString([]byte{bt})))
			}
			fmt.Print("Hex-STRING: " + strings.Join(strBytes, " ") + "\n")
		} else {
			fmt.Printf("STRING: %s\n", string(b))
		}
	case gosnmp.ObjectIdentifier:
		fmt.Printf("OID: %s\n", pdu.Value)
	case gosnmp.TimeTicks:
		fmt.Print(pdu.Value, "\n")
	case gosnmp.Counter32:
		fmt.Printf("Counter32: %d\n", pdu.Value.(uint))
	case gosnmp.Counter64:
		fmt.Printf("Counter64: %d\n", pdu.Value.(uint64))
	case gosnmp.Integer:
		fmt.Printf("INTEGER: %d\n", pdu.Value.(int))
	case gosnmp.Gauge32:
		fmt.Printf("Gauge32: %d\n", pdu.Value.(uint))
	case gosnmp.IPAddress:
		fmt.Printf("IpAddress: %s\n", pdu.Value.(string))
	default:
		fmt.Printf("TYPE %d: %d\n", pdu.Type, gosnmp.ToBigInt(pdu.Value))
	}

	return nil
}
