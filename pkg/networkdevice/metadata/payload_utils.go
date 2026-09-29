// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package metadata

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

// BatchPayloads batch NDM metadata payloads
func BatchPayloads(integration integrations.Integration, namespace string, subnet string, collectTime time.Time, batchSize int, devices []DeviceMetadata, interfaces []InterfaceMetadata, ipAddresses []IPAddressMetadata, topologyLinks []TopologyLinkMetadata, vpnTunnels []VPNTunnelMetadata, netflowExporters []NetflowExporter, diagnoses []DiagnosisMetadata) []NetworkDevicesMetadata {
	return BatchPayloadsWithWirelessInterfaces(integration, namespace, subnet, collectTime, batchSize, devices, interfaces, nil, ipAddresses, topologyLinks, vpnTunnels, netflowExporters, diagnoses)
}

// BatchPayloadsWithWirelessInterfaces batches every NDM resource with wireless
// interfaces sharing the same resource limit as the existing metadata types.
func BatchPayloadsWithWirelessInterfaces(integration integrations.Integration, namespace string, subnet string, collectTime time.Time, batchSize int, devices []DeviceMetadata, interfaces []InterfaceMetadata, wirelessInterfaces []WirelessInterfaceMetadata, ipAddresses []IPAddressMetadata, topologyLinks []TopologyLinkMetadata, vpnTunnels []VPNTunnelMetadata, netflowExporters []NetflowExporter, diagnoses []DiagnosisMetadata) []NetworkDevicesMetadata {
	var payloads []NetworkDevicesMetadata
	var resourceCount int

	curPayload := newNetworkDevicesMetadata(integration, namespace, subnet, collectTime)

	for _, deviceMetadata := range devices {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Devices = append(curPayload.Devices, deviceMetadata)
	}

	for _, interfaceMetadata := range interfaces {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Interfaces = append(curPayload.Interfaces, interfaceMetadata)
	}

	for _, wirelessInterface := range wirelessInterfaces {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.WirelessInterfaces = append(curPayload.WirelessInterfaces, wirelessInterface)
	}

	for _, ipAddress := range ipAddresses {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.IPAddresses = append(curPayload.IPAddresses, ipAddress)
	}

	for _, linkMetadata := range topologyLinks {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Links = append(curPayload.Links, linkMetadata)
	}

	for _, vpnTunnel := range vpnTunnels {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.VPNTunnels = append(curPayload.VPNTunnels, vpnTunnel)
	}

	for _, netflowExporter := range netflowExporters {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.NetflowExporters = append(curPayload.NetflowExporters, netflowExporter)
	}

	for _, diagnosis := range diagnoses {
		payloads, curPayload, resourceCount = appendToPayloads(integration, namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Diagnoses = append(curPayload.Diagnoses, diagnosis)
	}
	payloads = append(payloads, curPayload)
	return payloads
}

// DeviceID returns the SNMP integration identity used by NDM metadata and metrics.
func DeviceID(namespace, ipAddress string) string {
	return namespace + ":" + ipAddress
}

// DeviceIDTags are the sorted inputs used to resolve an SNMP device identity.
func DeviceIDTags(namespace, ipAddress string) []string {
	return []string{"device_namespace:" + namespace, "snmp_device:" + ipAddress}
}

// InterfaceID returns the IF-MIB interface identity within a device.
func InterfaceID(deviceID string, index int32) string {
	return fmt.Sprintf("%s:%d", deviceID, index)
}

// InterfaceIDTags correlate an IF-MIB interface with its metric series.
func InterfaceIDTags(name string, index int32) []string {
	return []string{"interface:" + name, fmt.Sprintf("interface_index:%d", index)}
}

// DeviceResourceTag associates a metric series with an NDM device resource.
func DeviceResourceTag(deviceID string) string {
	return "dd.internal.resource:ndm_device:" + deviceID
}

// InterfaceResourceTag associates a metric series with an NDM interface resource.
func InterfaceResourceTag(deviceID string, index int32) string {
	return "dd.internal.resource:ndm_interface:" + InterfaceID(deviceID, index)
}

// BatchDeviceScan batches a bunch of DeviceOID entries across multiple NetworkDevicesMetadata payloads.
func BatchDeviceScan(namespace string, collectTime time.Time, batchSize int, deviceOIDs []*DeviceOID) []NetworkDevicesMetadata {
	var payloads []NetworkDevicesMetadata
	var resourceCount int

	curPayload := newNetworkDevicesMetadata("snmp", namespace, "", collectTime)

	for _, oid := range deviceOIDs {
		payloads, curPayload, resourceCount = appendToPayloads("snmp", namespace, "", collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.DeviceOIDs = append(curPayload.DeviceOIDs, *oid)
	}
	payloads = append(payloads, curPayload)
	return payloads
}

func newNetworkDevicesMetadata(integration integrations.Integration, namespace string, subnet string, collectTime time.Time) NetworkDevicesMetadata {
	return NetworkDevicesMetadata{
		Subnet:           subnet,
		Namespace:        namespace,
		Integration:      integration,
		CollectTimestamp: collectTime.Unix(),
	}
}

func appendToPayloads(integration integrations.Integration, namespace string, subnet string, collectTime time.Time, batchSize int, resourceCount int, payloads []NetworkDevicesMetadata, payload NetworkDevicesMetadata) ([]NetworkDevicesMetadata, NetworkDevicesMetadata, int) {
	if resourceCount == batchSize {
		payloads = append(payloads, payload)
		payload = newNetworkDevicesMetadata(integration, namespace, subnet, collectTime)
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
