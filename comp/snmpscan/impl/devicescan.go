// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/batchsize"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
	"github.com/DataDog/datadog-agent/pkg/util/log"

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
	bulkMaxRep := scanParams.BulkMaxRepetitions
	if bulkMaxRep <= 0 {
		bulkMaxRep = defaultBulkMaxRepetitions
	}
	err = s.runDeviceScan(ctx, snmp, namespace, deviceID,
		scanParams.CallInterval, scanParams.MaxCallCount, bulkMaxRep)
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

// comparisonModeEnabled enables running both scan methods for comparison testing.
// Set to true for testing, false for production.
const comparisonModeEnabled = true

func (s snmpScannerImpl) runDeviceScan(
	ctx context.Context,
	snmpConnection *gosnmp.GoSNMP,
	deviceNamespace string,
	deviceID string,
	callInterval time.Duration,
	maxCallCount int,
	bulkMaxRep int,
) error {
	if comparisonModeEnabled {
		return s.runDeviceScanComparison(ctx, snmpConnection, callInterval, maxCallCount, bulkMaxRep)
	}

	// Execute the scan using GetBulk-based walk.
	// This approach never sends fabricated OIDs to devices, which:
	// - Prevents infinite loops on devices that respond incorrectly to non-existent OIDs
	// - Prevents crashes on devices that can't handle malformed table indices
	pdus, err := gatherPDUsWithBulk(ctx, snmpConnection, callInterval, maxCallCount, bulkMaxRep)
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

// scanResult holds the results of a scan method for comparison
type scanResult struct {
	name     string
	duration time.Duration
	oidCount int
	err      error
}

// runDeviceScanComparison runs both scan methods and prints comparison results.
// This is for testing purposes only - it does not send data to the backend.
func (s snmpScannerImpl) runDeviceScanComparison(
	ctx context.Context,
	snmpConnection *gosnmp.GoSNMP,
	callInterval time.Duration,
	maxCallCount int,
	bulkMaxRep int,
) error {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("SCAN COMPARISON MODE (testing only - no data sent)")
	fmt.Println(strings.Repeat("=", 60))

	var results []scanResult

	// Run GetBulk-based scan FIRST (safer - won't crash devices)
	fmt.Println("\n[1/2] Running GetBulk-based scan (new, safe approach)...")
	bulkStart := time.Now()
	bulkPDUs, bulkErr := gatherPDUsWithBulk(ctx, snmpConnection, callInterval, maxCallCount, bulkMaxRep)
	bulkDuration := time.Since(bulkStart)

	bulkResult := scanResult{
		name:     "GetBulk (new)",
		duration: bulkDuration,
		oidCount: len(bulkPDUs),
		err:      bulkErr,
	}
	results = append(results, bulkResult)

	if bulkErr != nil {
		fmt.Printf("      ERROR: %v\n", bulkErr)
	} else {
		fmt.Printf("      Completed: %d OIDs in %v\n", len(bulkPDUs), bulkDuration)
	}

	// Run old SkipOIDRowsNaive-based scan SECOND (may crash device or loop)
	fmt.Println("\n[2/2] Running SkipOIDRowsNaive-based scan (old approach)...")
	fmt.Println("      WARNING: This may crash the device or cause infinite loops!")
	oldStart := time.Now()
	oldPDUs, oldErr := gatherPDUs(ctx, snmpConnection, callInterval, maxCallCount)
	oldDuration := time.Since(oldStart)

	oldResult := scanResult{
		name:     "SkipOIDRowsNaive (old)",
		duration: oldDuration,
		oidCount: len(oldPDUs),
		err:      oldErr,
	}
	results = append(results, oldResult)

	if oldErr != nil {
		fmt.Printf("      ERROR: %v\n", oldErr)
	} else {
		fmt.Printf("      Completed: %d OIDs in %v\n", len(oldPDUs), oldDuration)
	}

	// Print comparison summary
	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("COMPARISON SUMMARY")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-25s | %-10s | %-15s | %s\n", "Method", "OIDs", "Duration", "Status")
	fmt.Println(strings.Repeat("-", 60))

	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = "FAILED"
		}
		fmt.Printf("%-25s | %-10d | %-15v | %s\n", r.name, r.oidCount, r.duration.Round(time.Millisecond), status)
	}
	fmt.Println(strings.Repeat("-", 60))

	// Print detailed error info if any scan failed
	for _, r := range results {
		if r.err != nil {
			fmt.Printf("\n%s error details:\n  %v\n", r.name, r.err)
		}
	}

	fmt.Println("\nNote: No scan data was sent to the backend (comparison mode).")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	// Return nil even if scans failed - we want to see the comparison output
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

// defaultBulkMaxRepetitions is the starting max-repetitions value for GetBulk
// during a device scan. Matches net-snmp's `snmpbulkwalk -Cr` default.
const defaultBulkMaxRepetitions = 10

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
//   - Cannot cause infinite loops - tracks visited OIDs
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
	visitedOIDs := make(map[string]bool)

	// Start from the beginning of the MIB tree
	oid := ".0.0"
	requests := 0
	maxRepOpt := batchsize.NewOptimizer(bulkMaxRep)

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
		maxRep := uint32(maxRepOpt.BatchSize())
		response, err := snmp.GetBulk([]string{oid}, 0, maxRep)
		if err != nil {
			shouldRetry := maxRepOpt.OnFailure()
			log.Debugf("SNMP scan GetBulk at OID %s with max-rep %d failed, new max-rep is %d",
				oid, maxRep, maxRepOpt.BatchSize())
			if shouldRetry {
				// Retry against the same OID at a smaller max-repetitions.
				continue
			}
			return result, fmt.Errorf("GetBulk error at OID %s (max-rep=%d): %w", oid, maxRep, err)
		}
		oldMaxRep := maxRepOpt.BatchSize()
		maxRepOpt.OnSuccess()
		if newMaxRep := maxRepOpt.BatchSize(); newMaxRep != oldMaxRep {
			log.Debugf("SNMP scan GetBulk at OID %s with max-rep %d success, new max-rep is %d",
				oid, oldMaxRep, newMaxRep)
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
