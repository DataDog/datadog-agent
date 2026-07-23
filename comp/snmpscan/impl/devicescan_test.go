// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package snmpscanimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discardPDU is an emit callback that drops PDUs, for walk tests that only
// care about request behavior rather than collected results.
func discardPDU(*gosnmp.SnmpPDU) error { return nil }

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

	err := gatherPDUsWithBulk(ctx, snmp, discardPDU, 0, 0, defaultBulkMaxRepetitions)
	assert.ErrorIs(t, err, context.Canceled)
}

// fakeBulkGetter is a scripted GetBulk for testing adaptive behavior.
type fakeBulkGetter struct {
	responses []bulkResponse
	calls     []bulkCall
}

type bulkCall struct {
	oid    string
	maxRep uint32
}

type bulkResponse struct {
	packet *gosnmp.SnmpPacket
	err    error
}

func (f *fakeBulkGetter) GetBulk(oids []string, _ uint8, maxRep uint32) (*gosnmp.SnmpPacket, error) {
	f.calls = append(f.calls, bulkCall{oid: oids[0], maxRep: maxRep})
	if len(f.calls) > len(f.responses) {
		return nil, errors.New("fakeBulkGetter: no more canned responses")
	}
	resp := f.responses[len(f.calls)-1]
	return resp.packet, resp.err
}

// endOfMibPacket returns a packet that immediately ends the walk.
func endOfMibPacket() *gosnmp.SnmpPacket {
	return &gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{Name: ".0.0", Type: gosnmp.EndOfMibView},
		},
	}
}

func TestGatherPDUsWithBulk_AdaptsMaxRepOnFailure(t *testing.T) {
	// First call at max-rep=10 times out; optimizer halves to 5; second call
	// succeeds. Verify the OID didn't advance and the second call used a
	// smaller max-rep.
	fake := &fakeBulkGetter{
		responses: []bulkResponse{
			{err: errors.New("request timeout")},
			{packet: endOfMibPacket()},
		},
	}

	err := gatherPDUsWithBulk(context.Background(), fake, discardPDU, 0, 0, 10)
	require.NoError(t, err)

	require.Len(t, fake.calls, 2)
	assert.Equal(t, ".0.0", fake.calls[0].oid)
	assert.Equal(t, ".0.0", fake.calls[1].oid, "OID should not advance on retry")
	assert.Equal(t, uint32(10), fake.calls[0].maxRep)
	assert.Equal(t, uint32(5), fake.calls[1].maxRep, "max-rep should halve on failure")
}

func TestGatherPDUsWithBulk_GivesUpWhenMaxRepCannotShrink(t *testing.T) {
	// Every call fails. The optimizer halves down to 1 and then has nowhere
	// further to go, so OnFailure returns false and gatherPDUsWithBulk
	// surfaces the error.
	timeoutErr := errors.New("request timeout")
	fake := &fakeBulkGetter{
		responses: []bulkResponse{
			{err: timeoutErr},
			{err: timeoutErr},
			{err: timeoutErr},
			{err: timeoutErr},
			{err: timeoutErr}, // floor at 1
		},
	}

	err := gatherPDUsWithBulk(context.Background(), fake, discardPDU, 0, 0, 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request timeout")
	// 4 → 2 → 1 → still 1 (OnFailure returns false): 3 calls.
	assert.GreaterOrEqual(t, len(fake.calls), 2)
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

	// 2 scalar OIDs (each unique) + 1 ifDescr row + 1 ifType row = 4
	assert.Equal(t, 4, len(result))

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

// collectOIDs sums the DeviceOIDs across the payloads a flusher sent.
func collectOIDs(payloads []metadata.NetworkDevicesMetadata) int {
	total := 0
	for _, p := range payloads {
		total += len(p.DeviceOIDs)
	}
	return total
}

func TestOIDFlusherCountBased(t *testing.T) {
	var sent []metadata.NetworkDevicesMetadata
	send := func(p metadata.NetworkDevicesMetadata) error {
		sent = append(sent, p)
		return nil
	}

	// flushInterval 0 so only the count threshold fires.
	flusher := newOIDFlusher("default", 100, 0, send)
	for i := 0; i < 250; i++ {
		assert.NoError(t, flusher.add(&metadata.DeviceOID{OID: "1.2.3"}))
	}
	// Two flushes happened mid-walk (at 100 and 200), so results stream out
	// before the scan finishes.
	assert.Len(t, sent, 2)
	assert.NoError(t, flusher.flush())
	// The final flush reports the remaining 50 OIDs; nothing is dropped.
	assert.Len(t, sent, 3)
	assert.Equal(t, 250, collectOIDs(sent))
}

func TestOIDFlusherEndOnly(t *testing.T) {
	var sent []metadata.NetworkDevicesMetadata
	send := func(p metadata.NetworkDevicesMetadata) error {
		sent = append(sent, p)
		return nil
	}

	// A count threshold larger than the OID count and no time threshold means
	// no mid-walk flush.
	flusher := newOIDFlusher("default", 1000, 0, send)
	for i := 0; i < 50; i++ {
		assert.NoError(t, flusher.add(&metadata.DeviceOID{OID: "1.2.3"}))
	}
	assert.Empty(t, sent)
	assert.NoError(t, flusher.flush())
	assert.Len(t, sent, 1)
	assert.Equal(t, 50, collectOIDs(sent))
}

func TestOIDFlusherEmptyFlushNoOp(t *testing.T) {
	called := false
	flusher := newOIDFlusher("default", 100, 0, func(metadata.NetworkDevicesMetadata) error {
		called = true
		return nil
	})
	assert.NoError(t, flusher.flush())
	assert.False(t, called)
}

// dataPacket returns a successful packet carrying the given OIDs as octet strings.
func dataPacket(oids ...string) *gosnmp.SnmpPacket {
	vars := make([]gosnmp.SnmpPDU, 0, len(oids))
	for _, oid := range oids {
		vars = append(vars, gosnmp.SnmpPDU{Name: oid, Type: gosnmp.OctetString, Value: "x"})
	}
	return &gosnmp.SnmpPacket{Variables: vars}
}

func TestGatherPDUsWithBulk_StopsOnNonAdvancingOID(t *testing.T) {
	// A misbehaving device that returns the same OID again (instead of a
	// strictly greater one) must be detected as stuck rather than looping
	// forever. This replaces the old visited-OID map with an O(1) monotonic
	// check.
	fake := &fakeBulkGetter{
		responses: []bulkResponse{
			{packet: dataPacket("1.3.6.1.2.1.1.1.0")},
			{packet: dataPacket("1.3.6.1.2.1.1.1.0")}, // repeat - does not advance
		},
	}

	err := gatherPDUsWithBulk(context.Background(), fake, discardPDU, 0, 0, defaultBulkMaxRepetitions)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not advance")
}

func TestGatherPDUsWithBulk_TreatsSNMPErrorAsFailure(t *testing.T) {
	// A packet with a non-NoError status must back the batch size off just
	// like a transport error, then a clean packet lets the walk finish.
	fake := &fakeBulkGetter{
		responses: []bulkResponse{
			{packet: &gosnmp.SnmpPacket{Error: gosnmp.TooBig}},
			{packet: endOfMibPacket()},
		},
	}

	err := gatherPDUsWithBulk(context.Background(), fake, discardPDU, 0, 0, 10)
	require.NoError(t, err)
	require.Len(t, fake.calls, 2)
	assert.Equal(t, uint32(10), fake.calls[0].maxRep)
	assert.Equal(t, uint32(5), fake.calls[1].maxRep, "max-rep should halve after an SNMP error")
}
