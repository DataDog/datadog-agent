// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"context"
	"errors"
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
		return fmt.Errorf("unable to send in progress status: %w", err)
	}
	if err = snmp.Connect(); err != nil {
		errs := []error{err}

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
			errs = append(errs, fmt.Errorf("unable to send error status: %w", sendErr))
		}
		return gosnmplib.NewConnectionError(
			fmt.Errorf("unable to connect to SNMP agent on %s:%d: %w",
				snmp.LocalAddr, snmp.Port, errors.Join(errs...)),
		)
	}
	err = s.runDeviceScan(ctx, snmp, namespace, deviceID,
		scanParams.CallInterval, scanParams.MaxCallCount)
	if err != nil {
		errs := []error{err}

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
			errs = append(errs, fmt.Errorf("unable to send error status: %w", sendErr))
		}
		return fmt.Errorf("unable to perform device scan: %w", errors.Join(errs...))
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
		return fmt.Errorf("unable to send completed status: %w", err)
	}
	return nil
}

func (s snmpScannerImpl) runDeviceScan(
	ctx context.Context,
	snmpConnection *gosnmp.GoSNMP,
	deviceNamespace string,
	deviceID string,
	callInterval time.Duration,
	maxCallCount int,
) error {
	// Execute the scan using GetBulk-based walk.
	// This approach never sends fabricated OIDs to devices, which:
	// - Prevents infinite loops on devices that respond incorrectly to non-existent OIDs
	// - Prevents crashes on devices that can't handle malformed table indices
	pdus, err := gatherPDUsWithBulk(ctx, snmpConnection, callInterval, maxCallCount)
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

// gatherPDUs returns PDUs from the given SNMP device that should cover every
// scalar value and at least one row of every table.
//
// Deprecated: This function uses SkipOIDRowsNaive which fabricates OIDs that may
// not exist on the device. This can cause infinite loops on devices that respond
// incorrectly to non-existent OIDs, or crashes on devices that can't handle
// malformed table indices. Use gatherPDUsWithBulk instead.
func gatherPDUs(ctx context.Context, snmp *gosnmp.GoSNMP, callInterval time.Duration, maxCallCount int) ([]*gosnmp.SnmpPDU, error) {
	var pdus []*gosnmp.SnmpPDU
	err := gosnmplib.ConditionalWalk(
		ctx,
		snmp,
		"",
		false,
		callInterval,
		maxCallCount,
		func(dataUnit gosnmp.SnmpPDU) (string, error) {
			pdus = append(pdus, &dataUnit)
			return gosnmplib.SkipOIDRowsNaive(dataUnit.Name), nil
		})
	if err != nil {
		return nil, err
	}
	return pdus, nil
}

const defaultBulkBatchSize = 50

// gatherPDUsWithBulk returns PDUs from the given SNMP device using GetBulk operations.
// It walks the entire MIB tree but filters results to keep only one row per column.
//
// Key safety properties:
//   - Never sends fabricated OIDs to the device - only OIDs the device has returned
//   - Cannot cause infinite loops - tracks visited OIDs
//   - Cannot crash devices - never sends malformed table indices
//
// Trade-off: May be slower than gatherPDUs for devices with large tables (1000+ rows)
// because it retrieves all rows before filtering.
func gatherPDUsWithBulk(ctx context.Context, snmp *gosnmp.GoSNMP, callInterval time.Duration, maxCallCount int) ([]*gosnmp.SnmpPDU, error) {
	var result []*gosnmp.SnmpPDU
	seenColumns := make(map[string]bool)
	visitedOIDs := make(map[string]bool)

	// Start from the beginning of the MIB tree
	oid := ".0.0"
	requests := 0
	batchSize := uint32(defaultBulkBatchSize)

	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if callInterval > 0 {
			time.Sleep(callInterval)
		}

		requests++
		if maxCallCount > 0 && requests >= maxCallCount {
			return result, fmt.Errorf("exceeded maximum request limit (%d)", maxCallCount)
		}

		// Use GetBulk with REAL OIDs only (never fabricated)
		response, err := snmp.GetBulk([]string{oid}, 0, batchSize)
		if err != nil {
			return result, fmt.Errorf("GetBulk error at OID %s: %w", oid, err)
		}

		if len(response.Variables) == 0 {
			// No more data
			break
		}

		var lastOID string

		for _, pdu := range response.Variables {
			// End conditions
			if pdu.Type == gosnmp.EndOfMibView ||
				pdu.Type == gosnmp.NoSuchObject ||
				pdu.Type == gosnmp.NoSuchInstance {
				return result, nil
			}

			// Cycle detection - if we've seen this OID before, we're in a loop
			if visitedOIDs[pdu.Name] {
				return result, fmt.Errorf("cycle detected: OID %s already visited", pdu.Name)
			}
			visitedOIDs[pdu.Name] = true

			lastOID = pdu.Name

			// Column filtering - keep first row of each "column"
			columnSig := gosnmplib.ExtractColumnSignature(pdu.Name)
			if !seenColumns[columnSig] {
				seenColumns[columnSig] = true
				pduCopy := pdu // Copy to avoid aliasing issues
				result = append(result, &pduCopy)
			}
		}

		// Stuck detection - if we didn't advance, something is wrong
		if lastOID == "" || lastOID == oid {
			return result, fmt.Errorf("walk stuck at OID %s", oid)
		}

		// Use the last OID from the batch as the starting point for the next request.
		// This OID came directly from the device, so it's always safe to use.
		oid = lastOID
	}

	return result, nil
}
