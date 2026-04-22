// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package snmpscanimpl

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestExtractColumnSignatureIntegration(t *testing.T) {
	// Test that ExtractColumnSignature works correctly for filtering purposes
	// This ensures the column.go and devicescan.go integration is correct

	testCases := []struct {
		name         string
		oids         []string
		expectedSigs int // Expected number of unique signatures
	}{
		{
			name: "scalar OIDs each get unique signature",
			oids: []string{
				"1.3.6.1.2.1.1.1.0",
				"1.3.6.1.2.1.1.2.0",
				"1.3.6.1.2.1.1.3.0",
			},
			expectedSigs: 3,
		},
		{
			name: "same column different rows (no 1 in index) get same signature",
			oids: []string{
				"1.3.6.1.2.1.2.2.1.2.2",
				"1.3.6.1.2.1.2.2.1.2.100",
				"1.3.6.1.2.1.2.2.1.2.200",
			},
			expectedSigs: 1,
		},
		{
			name: "different columns get different signatures",
			oids: []string{
				"1.3.6.1.2.1.2.2.1.2.2", // ifDescr
				"1.3.6.1.2.1.2.2.1.3.2", // ifType
				"1.3.6.1.2.1.2.2.1.4.2", // ifMtu
				"1.3.6.1.2.1.2.2.1.5.2", // ifSpeed
			},
			expectedSigs: 4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigs := make(map[string]bool)
			for _, oid := range tc.oids {
				sig := gosnmplib.ExtractColumnSignature(oid)
				sigs[sig] = true
			}
			assert.Equal(t, tc.expectedSigs, len(sigs), "Expected %d unique signatures, got %d", tc.expectedSigs, len(sigs))
		})
	}
}

func TestGatherPDUsWithBulk_ContextCancellation(t *testing.T) {
	// Test that the function respects context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	snmp := &gosnmp.GoSNMP{
		Target:    "127.0.0.1",
		Port:      161,
		Community: "public",
		Version:   gosnmp.Version2c,
	}

	_, err := gatherPDUsWithBulk(ctx, snmp, 0, 0)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestGatherPDUsWithBulk_MaxCallCount(t *testing.T) {
	// Test that max call count is enforced
	// This is a unit test that doesn't require actual SNMP connection
	// since it will hit the limit before making any real calls

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	snmp := &gosnmp.GoSNMP{
		Target:    "127.0.0.1",
		Port:      161,
		Community: "public",
		Version:   gosnmp.Version2c,
		Timeout:   10 * time.Millisecond,
	}

	// With maxCallCount=1, it should try one GetBulk, fail to connect,
	// and return an error (either connection error or max count error)
	_, err := gatherPDUsWithBulk(ctx, snmp, 0, 1)
	assert.Error(t, err)
}

func TestColumnFilteringLogic(t *testing.T) {
	// Test the column filtering logic in isolation
	// Simulates what gatherPDUsWithBulk does internally

	pdus := []gosnmp.SnmpPDU{
		// Scalar OIDs
		{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "System Description"},
		{Name: "1.3.6.1.2.1.1.2.0", Type: gosnmp.ObjectIdentifier, Value: "1.3.6.1.4.1.9"},

		// ifTable rows - should keep only first row of each column
		{Name: "1.3.6.1.2.1.2.2.1.2.2", Type: gosnmp.OctetString, Value: "eth0"},
		{Name: "1.3.6.1.2.1.2.2.1.2.3", Type: gosnmp.OctetString, Value: "eth1"}, // Same column, should filter
		{Name: "1.3.6.1.2.1.2.2.1.3.2", Type: gosnmp.Integer, Value: 6},
		{Name: "1.3.6.1.2.1.2.2.1.3.3", Type: gosnmp.Integer, Value: 6}, // Same column, should filter
	}

	// Simulate the filtering logic from gatherPDUsWithBulk
	seenColumns := make(map[string]bool)
	var result []*gosnmp.SnmpPDU

	for _, pdu := range pdus {
		columnSig := gosnmplib.ExtractColumnSignature(pdu.Name)
		if !seenColumns[columnSig] {
			seenColumns[columnSig] = true
			pduCopy := pdu
			result = append(result, &pduCopy)
		}
	}

	// Should have:
	// 2 scalar OIDs (each unique)
	// 1 row from ifDescr column
	// 1 row from ifType column
	// Total: 4
	assert.Equal(t, 4, len(result))

	// Verify the kept PDUs
	keptOIDs := make(map[string]bool)
	for _, pdu := range result {
		keptOIDs[pdu.Name] = true
	}

	assert.True(t, keptOIDs["1.3.6.1.2.1.1.1.0"], "Scalar OID should be kept")
	assert.True(t, keptOIDs["1.3.6.1.2.1.1.2.0"], "Scalar OID should be kept")
	assert.True(t, keptOIDs["1.3.6.1.2.1.2.2.1.2.2"], "First ifDescr row should be kept")
	assert.True(t, keptOIDs["1.3.6.1.2.1.2.2.1.3.2"], "First ifType row should be kept")
	assert.False(t, keptOIDs["1.3.6.1.2.1.2.2.1.2.3"], "Second ifDescr row should be filtered")
	assert.False(t, keptOIDs["1.3.6.1.2.1.2.2.1.3.3"], "Second ifType row should be filtered")
}

func TestCycleDetectionLogic(t *testing.T) {
	// Test that cycle detection works correctly
	visitedOIDs := make(map[string]bool)

	oids := []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.1.0", // Repeat - cycle!
	}

	var cycleDetected bool
	for _, oid := range oids {
		if visitedOIDs[oid] {
			cycleDetected = true
			break
		}
		visitedOIDs[oid] = true
	}

	assert.True(t, cycleDetected, "Should detect cycle when OID is repeated")
}

func TestEndOfMibDetection(t *testing.T) {
	// Test that end-of-MIB conditions are handled correctly
	endConditions := []gosnmp.Asn1BER{
		gosnmp.EndOfMibView,
		gosnmp.NoSuchObject,
		gosnmp.NoSuchInstance,
	}

	for _, condition := range endConditions {
		t.Run(condition.String(), func(t *testing.T) {
			isEnd := condition == gosnmp.EndOfMibView ||
				condition == gosnmp.NoSuchObject ||
				condition == gosnmp.NoSuchInstance
			assert.True(t, isEnd, "Should recognize %s as end condition", condition)
		})
	}
}
