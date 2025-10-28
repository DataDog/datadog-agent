// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"context"
	"fmt"
	"time"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"

	"github.com/gosnmp/gosnmp"
)

func (s snmpScannerImpl) ScanDeviceAndSendData(ctx context.Context, connParams *snmpparse.SNMPConfig, namespace string, scanParams snmpscan.ScanParams) error {
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
			ScanType:   scanParams.ScanType,
		},
		CollectTimestamp: time.Now().Unix(),
		Namespace:        namespace,
	}
	if err = s.sendPayload(inProgressStatusPayload); err != nil {
		return fmt.Errorf("unable to send in progress status: %v", err)
	}
	if err = snmp.Connect(); err != nil {
		// Send an error status if we can't connect to the agent
		errorStatusPayload := metadata.NetworkDevicesMetadata{
			DeviceScanStatus: &metadata.ScanStatusMetadata{
				DeviceID:   deviceID,
				ScanStatus: metadata.ScanStatusError,
				ScanType:   scanParams.ScanType,
			},
			CollectTimestamp: time.Now().Unix(),
			Namespace:        namespace,
		}
		if sendErr := s.sendPayload(errorStatusPayload); sendErr != nil {
			return fmt.Errorf("unable to send error status: %v", sendErr)
		}
		return fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w", snmp.LocalAddr, snmp.Port, err)
	}
	err = s.runDeviceScan(ctx, snmp, namespace, deviceID, scanParams.CallInterval)
	if err != nil {
		// Send an error status if we can't scan the device
		errorStatusPayload := metadata.NetworkDevicesMetadata{
			DeviceScanStatus: &metadata.ScanStatusMetadata{
				DeviceID:   deviceID,
				ScanStatus: metadata.ScanStatusError,
				ScanType:   scanParams.ScanType,
			},
			CollectTimestamp: time.Now().Unix(),
			Namespace:        namespace,
		}
		if sendErr := s.sendPayload(errorStatusPayload); sendErr != nil {
			return fmt.Errorf("unable to send error status: %v", sendErr)
		}
		return fmt.Errorf("unable to perform device scan: %w", err)
	}
	// Send a completed status if the scan was successful
	completedStatusPayload := metadata.NetworkDevicesMetadata{
		DeviceScanStatus: &metadata.ScanStatusMetadata{
			DeviceID:   deviceID,
			ScanStatus: metadata.ScanStatusCompleted,
			ScanType:   scanParams.ScanType,
		},
		CollectTimestamp: time.Now().Unix(),
		Namespace:        namespace,
	}
	if err = s.sendPayload(completedStatusPayload); err != nil {
		return fmt.Errorf("unable to send completed status: %v", err)
	}
	return nil
}

func (s snmpScannerImpl) runDeviceScan(ctx context.Context, snmpConnection *gosnmp.GoSNMP, deviceNamespace string, deviceID string, callInterval time.Duration) error {
	// execute the scan
	pdus, err := gatherPDUs(ctx, snmpConnection, callInterval)
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
		err := s.sendPayload(payload)
		if err != nil {
			return err
		}
	}

	return nil
}

// gatherPDUs returns PDUs from the given SNMP device that should cover ever
// scalar value and at least one row of every table.
func gatherPDUs(ctx context.Context, snmp *gosnmp.GoSNMP, callInterval time.Duration) ([]*gosnmp.SnmpPDU, error) {
	var pdus []*gosnmp.SnmpPDU
	err := gosnmplib.ConditionalWalk(ctx, snmp, "", false, callInterval, func(dataUnit gosnmp.SnmpPDU) (string, error) {
		pdus = append(pdus, &dataUnit)
		return gosnmplib.SkipOIDRowsNaive(dataUnit.Name), nil
	})
	if err != nil {
		return nil, err
	}
	return pdus, nil
}
