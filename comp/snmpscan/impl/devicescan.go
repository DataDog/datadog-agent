// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
)

func (s snmpScannerImpl) RunDeviceScan(snmpConnection *gosnmp.GoSNMP, deviceNamespace string, deviceID string) error {
	// execute the scan
	pdus, err := gatherPDUs(snmpConnection)
	if err != nil {
		return err
	}

	var deviceOids []*metadata.DeviceOID
	for _, pdu := range pdus {
		record, err := metadata.DeviceOIDFromPDU(deviceID, pdu)
		if err != nil {
			s.log.Warnf("PDU parsing error: %v", err)
			continue
		}
		deviceOids = append(deviceOids, record)
	}

	metadataPayloads := metadata.BatchDeviceScan(deviceNamespace, time.Now(), metadata.PayloadMetadataBatchSize, deviceOids)
	for _, payload := range metadataPayloads {
		err := s.SendPayload(payload)
		if err != nil {
			return err
		}
	}

	return nil
}

// gatherPDUs returns PDUs from the given SNMP device that should cover ever
// scalar value and at least one row of every table.
func gatherPDUs(snmp *gosnmp.GoSNMP) ([]*gosnmp.SnmpPDU, error) {
	var pdus []*gosnmp.SnmpPDU
	err := gosnmplib.ConditionalWalk(snmp, "", false, func(dataUnit gosnmp.SnmpPDU) (string, error) {
		pdus = append(pdus, &dataUnit)
		return gosnmplib.SkipOIDRowsNaive(dataUnit.Name), nil
	})
	if err != nil {
		return nil, err
	}
	return pdus, nil
}

func (s snmpScannerImpl) ScanDeviceAndSendData(connParams *snmpparse.SNMPConfig, namespace string, scanType metadata.ScanType) error {
	// Establish connection
	snmp, err := snmpparse.NewSNMP(connParams, s.log)
	if err != nil {
		return err
	}
	deviceID := namespace + ":" + connParams.IPAddress
	// Since the snmp connection can take a while, start by sending an in progress status for the start of the scan
	// before connecting to the agent
	inProgressStatusPayload := metadata.NetworkDevicesMetadata{
		DeviceScanStatus: &metadata.ScanStatusMetadata{
			DeviceID:   deviceID,
			ScanStatus: metadata.ScanStatusInProgress,
			ScanType:   scanType,
		},
		CollectTimestamp: time.Now().Unix(),
		Namespace:        namespace,
	}
	if err = s.SendPayload(inProgressStatusPayload); err != nil {
		return fmt.Errorf("unable to send in progress status: %v", err)
	}
	if err = snmp.Connect(); err != nil {
		// Send an error status if we can't connect to the agent
		errorStatusPayload := metadata.NetworkDevicesMetadata{
			DeviceScanStatus: &metadata.ScanStatusMetadata{
				DeviceID:   deviceID,
				ScanStatus: metadata.ScanStatusError,
				ScanType:   scanType,
			},
			CollectTimestamp: time.Now().Unix(),
			Namespace:        namespace,
		}
		if err = s.SendPayload(errorStatusPayload); err != nil {
			return fmt.Errorf("unable to send error status: %v", err)
		}
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}
	err = s.RunDeviceScan(snmp, namespace, deviceID)
	if err != nil {
		// Send an error status if we can't scan the device
		errorStatusPayload := metadata.NetworkDevicesMetadata{
			DeviceScanStatus: &metadata.ScanStatusMetadata{
				DeviceID:   deviceID,
				ScanStatus: metadata.ScanStatusError,
				ScanType:   scanType,
			},
			CollectTimestamp: time.Now().Unix(),
			Namespace:        namespace,
		}
		if err = s.SendPayload(errorStatusPayload); err != nil {
			return fmt.Errorf("unable to send error status: %v", err)
		}
		return fmt.Errorf("unable to perform device scan: %v", err)
	}
	// Send a completed status if the scan was successful
	completedStatusPayload := metadata.NetworkDevicesMetadata{
		DeviceScanStatus: &metadata.ScanStatusMetadata{
			DeviceID:   deviceID,
			ScanStatus: metadata.ScanStatusCompleted,
			ScanType:   scanType,
		},
		CollectTimestamp: time.Now().Unix(),
		Namespace:        namespace,
	}
	if err = s.SendPayload(completedStatusPayload); err != nil {
		return fmt.Errorf("unable to send completed status: %v", err)
	}
	return nil
}
