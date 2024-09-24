// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

// BatchPayloads batch NDM metadata payloads
func BatchPayloads(namespace string, subnet string, collectTime time.Time, batchSize int, devices []DeviceMetadata, interfaces []InterfaceMetadata, ipAddresses []IPAddressMetadata, topologyLinks []TopologyLinkMetadata, netflowExporters []NetflowExporter, diagnoses []DiagnosisMetadata) []NetworkDevicesMetadata {

	var payloads []NetworkDevicesMetadata
	var resourceCount int

	curPayload := newNetworkDevicesMetadata(namespace, subnet, collectTime)

	for _, deviceMetadata := range devices {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Devices = append(curPayload.Devices, deviceMetadata)
	}

	for _, interfaceMetadata := range interfaces {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Interfaces = append(curPayload.Interfaces, interfaceMetadata)
	}

	for _, ipAddress := range ipAddresses {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.IPAddresses = append(curPayload.IPAddresses, ipAddress)
	}

	for _, linkMetadata := range topologyLinks {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Links = append(curPayload.Links, linkMetadata)
	}

	for _, netflowExporter := range netflowExporters {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.NetflowExporters = append(curPayload.NetflowExporters, netflowExporter)
	}

	for _, diagnosis := range diagnoses {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Diagnoses = append(curPayload.Diagnoses, diagnosis)
	}
	payloads = append(payloads, curPayload)
	return payloads
}

// BatchDeviceScan batches a bunch of DeviceOID entries across multiple NetworkDevicesMetadata payloads.
func BatchDeviceScan(namespace string, collectTime time.Time, batchSize int, deviceOIDs []*DeviceOID) []NetworkDevicesMetadata {
	var payloads []NetworkDevicesMetadata
	var resourceCount int

	curPayload := newNetworkDevicesMetadata(namespace, "", collectTime)

	for _, oid := range deviceOIDs {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, "", collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.DeviceOIDs = append(curPayload.DeviceOIDs, *oid)
	}
	payloads = append(payloads, curPayload)
	return payloads
}

func newNetworkDevicesMetadata(namespace string, subnet string, collectTime time.Time) NetworkDevicesMetadata {
	return NetworkDevicesMetadata{
		Subnet:           subnet,
		Namespace:        namespace,
		CollectTimestamp: collectTime.Unix(),
	}
}

func appendToPayloads(namespace string, subnet string, collectTime time.Time, batchSize int, resourceCount int, payloads []NetworkDevicesMetadata, payload NetworkDevicesMetadata) ([]NetworkDevicesMetadata, NetworkDevicesMetadata, int) {
	if resourceCount == batchSize {
		payloads = append(payloads, payload)
		payload = newNetworkDevicesMetadata(namespace, subnet, collectTime)
		resourceCount = 0
	}
	resourceCount++
	return payloads, payload, resourceCount
}

// DeviceOIDFromPDU packages a gosnmp PDU as a DeviceOID
func DeviceOIDFromPDU(deviceID string, snmpPDU *gosnmp.SnmpPDU) (*DeviceOID, error) {
	pdu, err := gosnmplib.PDUFromSNMP(snmpPDU)
	stringType, stringValue, base64Value, err := GetTypeAndValueFromPDU(snmpPDU)
	if err != nil {
		return nil, err
	}
	return &DeviceOID{
		DeviceID:      deviceID,
		Oid:           pdu.OID,
		Type:          stringType,
		ValueAsString: stringValue,
		ValueAsBase64: base64Value,
	}, nil
}

// GetTypeAndValueFromPDU returns the type as a string, the value as a string and the value encoded in base64 and an error if any
func GetTypeAndValueFromPDU(pduVariable *gosnmp.SnmpPDU) (string, string, string, error) {
	typeConversion := func(value interface{}, typeName string) (string, string, string, error) {
		strValue := fmt.Sprintf("%v", value)
		base64Value := base64.StdEncoding.EncodeToString([]byte(strValue))
		return typeName, strValue, base64Value, nil
	}
	switch pduVariable.Type {
	case gosnmp.OctetString, gosnmp.BitString:
		bytesValue, ok := pduVariable.Value.([]byte)
		if !ok {
			return "", "", "", fmt.Errorf("oid %s: OctetString should be []byte type but got type `%T` and value `%v`", pduVariable.Name, pduVariable.Value, pduVariable.Value)
		}
		if !gosnmplib.IsStringPrintable(bytesValue) {
			var strBytes []string
			for _, bt := range bytesValue {
				strBytes = append(strBytes, strings.ToUpper(hex.EncodeToString([]byte{bt})))
			}
			return pduVariable.Type.String(), strings.Join(strBytes, " "), base64.StdEncoding.EncodeToString(bytesValue), nil
		}
		return pduVariable.Type.String(), string(bytesValue), base64.StdEncoding.EncodeToString(bytesValue), nil
	case gosnmp.Integer:
		return typeConversion(pduVariable.Value.(int), "Integer")
	case gosnmp.Counter32, gosnmp.Gauge32:
		return typeConversion(pduVariable.Value.(uint), pduVariable.Type.String())
	case gosnmp.TimeTicks:
		floatValue := float64(gosnmp.ToBigInt(pduVariable.Value).Int64())
		return typeConversion(floatValue, "TimeTicks")
	case gosnmp.Counter64:
		return typeConversion(pduVariable.Value.(uint64), "Counter64")
	case gosnmp.Uinteger32:
		return typeConversion(pduVariable.Value.(uint32), "Uinteger32")
	case gosnmp.OpaqueFloat:
		floatValue, ok := pduVariable.Value.(float32)
		if !ok {
			return "", "", "", fmt.Errorf("oid %s: OpaqueFloat should be float32 type but got type `%T` and value `%v`", pduVariable.Name, pduVariable.Value, pduVariable.Value)
		}
		return typeConversion(float64(floatValue), "OpaqueFloat")
	case gosnmp.OpaqueDouble:
		floatValue, ok := pduVariable.Value.(float64)
		if !ok {
			return "", "", "", fmt.Errorf("oid %s: OpaqueDouble should be float64 type but got type `%T` and value `%v`", pduVariable.Name, pduVariable.Value, pduVariable.Value)
		}
		return typeConversion(floatValue, "OpaqueDouble")
	case gosnmp.IPAddress, gosnmp.ObjectIdentifier:
		strValue, ok := pduVariable.Value.(string)
		if !ok {
			return "", "", "", fmt.Errorf("oid %s: %s should be string type but got type `%T` and value `%v`", pduVariable.Name, pduVariable.Type.String(), pduVariable.Value, pduVariable.Value)
		}
		if pduVariable.Type == gosnmp.ObjectIdentifier {
			strValue = strings.TrimLeft(strValue, ".")
		}
		return typeConversion(strValue, pduVariable.Type.String())
	case gosnmp.Boolean:
		boolValue, ok := pduVariable.Value.(bool)
		if !ok {
			return "", "", "", fmt.Errorf("oid %s: Boolean should be bool type but got type `%T` and value `%v`", pduVariable.Name, pduVariable.Value, pduVariable.Value)
		}
		return typeConversion(boolValue, "Boolean")
	case gosnmp.Null:
		return "Null", "null", base64.StdEncoding.EncodeToString([]byte("null")), nil
	case gosnmp.NoSuchObject:
		return "NoSuchObject", "noSuchObject", base64.StdEncoding.EncodeToString([]byte("noSuchObject")), nil
	case gosnmp.NoSuchInstance:
		return "NoSuchInstance", "noSuchInstance", base64.StdEncoding.EncodeToString([]byte("noSuchInstance")), nil
	case gosnmp.EndOfMibView:
		return "EndOfMibView", "endOfMibView", base64.StdEncoding.EncodeToString([]byte("endOfMibView")), nil
	default:
		return "", "", "", fmt.Errorf("oid %s: invalid type: %s", pduVariable.Name, pduVariable.Type.String())
	}
}
