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
	"github.com/DataDog/datadog-agent/pkg/snmp/batchsize"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/gosnmp/gosnmp"
)

// defaultBulkMaxRepetitions is the starting max-repetitions value for GetBulk
// during a device scan when none is configured.
const defaultBulkMaxRepetitions = 20

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

	// GetBulk is used for the scan, but it does not exist in SNMPv1, so fall
	// back to the legacy GetNext walk for v1 devices.
	useBulk := snmp.Version != gosnmp.Version1
	if !useBulk {
		s.log.Infof("device %s is SNMPv1, using GetNext for the scan (GetBulk unsupported)", deviceID)
	}

	err = s.runDeviceScan(ctx, snmp, namespace, deviceID, useBulk,
		scanParams.CallInterval, scanParams.MaxCallCount, defaultBulkMaxRepetitions)
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
	useBulk bool,
	callInterval time.Duration,
	maxCallCount int,
	bulkMaxRep int,
) error {
	// execute the scan
	var (
		pdus []*gosnmp.SnmpPDU
		err  error
	)
	if useBulk {
		pdus, err = gatherPDUsWithBulk(ctx, snmpConnection, callInterval, maxCallCount, bulkMaxRep)
	} else {
		pdus, err = gatherPDUs(ctx, snmpConnection, callInterval, maxCallCount)
	}
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
// Deprecated: this GetNext walk relies on SkipOIDRowsNaive, which fabricates
// OIDs that may not exist on the device and can cause infinite loops or crash
// some devices. It is kept only as the SNMPv1 fallback, since v1 has no
// GetBulk. Everything else uses gatherPDUsWithBulk.
func gatherPDUs(ctx context.Context, snmp *gosnmp.GoSNMP, callInterval time.Duration, maxCallCount int) ([]*gosnmp.SnmpPDU, error) {
	var pdus []*gosnmp.SnmpPDU
	err := gosnmplib.ConditionalWalk(
		ctx,
		snmp,
		"",
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

// bulkGetter is the GetBulk surface gatherPDUsWithBulk needs. Wrapping it in an
// interface lets tests substitute a fake without depending on a live gosnmp.
type bulkGetter interface {
	GetBulk(oids []string, nonRepeaters uint8, maxRepetitions uint32) (*gosnmp.SnmpPacket, error)
}

// gatherPDUsWithBulk returns PDUs from the given SNMP device using GetBulk operations.
// It walks the entire MIB tree but filters results to keep only one row per column.
//
// Key safety properties:
//   - Never sends fabricated OIDs to the device - only OIDs the device has returned
//   - Cannot cause infinite loops - every returned OID must be strictly after the
//     previous one, otherwise the walk stops
//   - Cannot crash devices - never sends malformed table indices
//
// Adaptive max-repetitions: on GetBulk error the value is halved and the same
// OID is retried, so a device that times out at a high value can still be
// walked. On success the value grows back toward bulkMaxRep.
//
// Trade-off: May be slower than gatherPDUs for devices with large tables (1000+ rows)
// because it retrieves all rows before filtering.
func gatherPDUsWithBulk(ctx context.Context, snmp bulkGetter, callInterval time.Duration, maxCallCount int, bulkMaxRep int) ([]*gosnmp.SnmpPDU, error) {
	var result []*gosnmp.SnmpPDU
	seenColumns := make(map[string]bool)

	// Start from the beginning of the MIB tree.
	oid := ".0.0"
	// prevInts is the parsed form of the last OID we accepted. SNMP walks are
	// strictly increasing, so comparing each returned OID against it detects
	// loops and non-advancing devices in O(1) memory - no need to remember
	// every OID we have seen.
	prevInts, err := gosnmplib.OIDToInts(oid)
	if err != nil {
		return nil, err
	}
	requests := 0
	maxRepOpt := batchsize.NewOptimizer(bulkMaxRep, "SNMP scan GetBulk")

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

		// Use GetBulk with REAL OIDs only (never fabricated).
		maxRep := uint32(maxRepOpt.BatchSize())
		response, err := snmp.GetBulk([]string{oid}, 0, maxRep)
		if err != nil || response.Error != gosnmp.NoError {
			// Both a transport error and a non-NoError SNMP status mean this
			// request failed; back the batch size off and retry the same OID.
			if maxRepOpt.OnFailure() {
				continue
			}
			if err != nil {
				return result, fmt.Errorf("GetBulk error at OID %s (max-rep=%d): %w", oid, maxRep, err)
			}
			return result, fmt.Errorf("GetBulk returned SNMP error %s at OID %s (max-rep=%d)", response.Error, oid, maxRep)
		}
		maxRepOpt.OnSuccess()

		if len(response.Variables) == 0 {
			// No more data.
			break
		}

		lastOID := oid

		for _, pdu := range response.Variables {
			// End conditions.
			if pdu.Type == gosnmp.EndOfMibView ||
				pdu.Type == gosnmp.NoSuchObject ||
				pdu.Type == gosnmp.NoSuchInstance {
				log.Debugf("SNMP scan walk reached end of MIB view at OID %s after %d requests", lastOID, requests)
				return result, nil
			}

			// Loop/stuck detection: each returned OID must be strictly after
			// the previous one. If it isn't, the device is looping or not
			// advancing, so stop.
			cur, err := gosnmplib.OIDToInts(pdu.Name)
			if err != nil {
				return result, err
			}
			if !gosnmplib.CmpOIDs(cur, prevInts).IsAfter() {
				log.Debugf("SNMP scan walk stopped: OID %s did not advance past %s", pdu.Name, lastOID)
				return result, fmt.Errorf("walk stuck: OID %s did not advance past %s", pdu.Name, lastOID)
			}
			prevInts = cur
			lastOID = pdu.Name

			// Column filtering - keep first row of each "column".
			columnSig := gosnmplib.ExtractColumnSignature(pdu.Name)
			if !seenColumns[columnSig] {
				seenColumns[columnSig] = true
				pduCopy := pdu // Copy to avoid aliasing issues.
				result = append(result, &pduCopy)
			}
		}

		// Use the last OID from the batch as the starting point for the next request.
		// This OID came directly from the device, so it's always safe to use.
		oid = lastOID
	}

	return result, nil
}
