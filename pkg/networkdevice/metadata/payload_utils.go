// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

import (
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
	if err != nil {
		return nil, err
	}
	return &DeviceOID{
		DeviceID: deviceID,
		OID:      pdu.OID,
		Type:     pdu.Type.String(),
		Value:    pdu.Value,
	}, nil
}
