// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
)

func TestCollectQBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)

	result := collect(sess, config{DeviceID: "default:1.2.3.4", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
	assert.Equal(t, int32(10), result.Entries[0].InterfaceIndex)
}

func TestCollectBridgeFallback(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(sess, config{DeviceID: "ns:10.0.0.1", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceBridge, result.Source)
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3d", result.Entries[0].MacAddress)
	assert.Equal(t, int32(20), result.Entries[0].InterfaceIndex)
}

func TestCollectFiltersInvalidRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	// learned unicast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)
	// multicast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.1.0.94.0.0.1", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.1.0.94.0.0.1", 3)
	// broadcast
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.255.255.255.255.255.255", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.255.255.255.255.255.255", 3)
	// zero MAC
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.0.0.0.0.0.0", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.0.0.0.0.0.0", 3)
	// zero port
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.70", 0)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.70", 3)
	// self(4) and mgmt(5) must not be treated as host locations.
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.80", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.80", 4)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.81", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.81", 5)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, "0a:14:1e:28:32:3c", result.Entries[0].MacAddress)
}

func TestCollectWithoutStatusTable(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	require.Len(t, result.Entries, 1)
	assert.Equal(t, int32(10), result.Entries[0].InterfaceIndex)
}

func TestCollectTruncatedDiscardsRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	for i := 1; i <= 5; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
	}

	result := collect(sess, config{DeviceID: "d", MaxEntries: 2, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
}

func TestCollectPartialQBridgeErrorDoesNotFallBackToBridge(t *testing.T) {
	inner := session.CreateFakeSession()
	inner.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	inner.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(&incompleteThenFail{
		FakeSession: inner,
		prefix:      oidDot1qTpFdbPort,
		first: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{
			Name:  oidDot1qTpFdbPort + ".1.10.20.30.40.50.60",
			Type:  gosnmp.Integer,
			Value: 1,
		}}},
	}, config{
		DeviceID:           "d",
		MaxEntries:         100,
		MaxDuration:        time.Second,
		BulkMaxRepetitions: 10,
	})
	assert.Equal(t, OutcomeError, result.Outcome)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, SourceBridge, result.Source)
}

func TestCollectStatusWalkErrorDiscardsRows(t *testing.T) {
	inner := session.CreateFakeSession()
	inner.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 10)
	inner.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	inner.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)
	inner.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(&failFirstPrefix{FakeSession: inner, prefix: oidDot1qTpFdbStatus}, config{
		DeviceID:           "d",
		MaxEntries:         100,
		MaxDuration:        time.Second,
		BulkMaxRepetitions: 10,
	})
	assert.Equal(t, OutcomeError, result.Outcome)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, SourceBridge, result.Source)
}

func TestCollectQBridgeFirstPageErrorDoesNotFallBack(t *testing.T) {
	inner := session.CreateFakeSession()
	inner.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	inner.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(&failFirstPrefix{FakeSession: inner, prefix: oidDot1qTpFdbPort}, config{
		DeviceID:           "d",
		MaxEntries:         100,
		MaxDuration:        time.Second,
		BulkMaxRepetitions: 10,
	})
	assert.Equal(t, OutcomeError, result.Outcome)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, SourceBridge, result.Source)
}

func TestCollectTruncatedQBridgeDoesNotFallBackToBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	for i := 1; i <= 3; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50."+itoa(i), 1)
	}
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Empty(t, result.Entries)
	assert.NotEqual(t, SourceBridge, result.Source)
}

func TestCollectPortMapTruncationDiscardsRows(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.1", 101)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 102)
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.3", 103)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 3)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 1, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
}

func TestCollectPopulatedBridgeKeepsSourceWhenFiltered(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceBridge, result.Source)
	assert.Empty(t, result.Entries)
}

func TestCollectPopulatedQBridgeDoesNotFallBackToBridge(t *testing.T) {
	sess := session.CreateFakeSession()
	// Q-BRIDGE rows exist but their bridge ports are unmapped.
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.3.1.10.20.30.40.50.60", 3)
	// BRIDGE-MIB would succeed if fallback ran.
	sess.SetInt("1.3.6.1.2.1.17.1.4.1.2.2", 20)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.2.10.20.30.40.50.61", 2)
	sess.SetInt("1.3.6.1.2.1.17.4.3.1.3.10.20.30.40.50.61", 3)

	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Equal(t, SourceQBridge, result.Source)
	assert.Empty(t, result.Entries)
}

func TestWalkRejectedRowsCountTowardLimit(t *testing.T) {
	prefix := oidDot1qTpFdbPort
	res := walkColumn(&fixedBulkSession{
		version: gosnmp.Version2c,
		packet: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
			{Name: prefix + ".1.1", Type: gosnmp.Null, Value: nil},
			{Name: prefix + ".1.2", Type: gosnmp.Null, Value: nil},
			{Name: prefix + ".1.3", Type: gosnmp.Integer, Value: 1},
		}},
	}, prefix, 10, 2, time.Time{})
	assert.Equal(t, reasonMaxEntries, res.reason)
	assert.ErrorIs(t, res.err, errWalkMaxEntries)
	assert.Empty(t, res.values)
}

func TestCollectEmptySuccess(t *testing.T) {
	sess := session.CreateFakeSession()
	result := collect(sess, config{DeviceID: "d", MaxEntries: 100, MaxDuration: time.Second, BulkMaxRepetitions: 10})
	assert.Equal(t, OutcomeSuccess, result.Outcome)
	assert.Empty(t, result.Entries)
}

func TestCollectUsesFixedEntryLimit(t *testing.T) {
	sess := session.CreateFakeSession()
	for i := 0; i <= maxEntries; i++ {
		sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30."+itoa(i/256)+"."+itoa(i%256), 1)
	}

	result := Collect(sess, "d", 10)
	assert.Equal(t, OutcomeTruncated, result.Outcome)
	assert.Equal(t, reasonMaxEntries, result.Reason)
	assert.Empty(t, result.Entries)
}

func TestWalkSNMPErrorStatusIsError(t *testing.T) {
	res := walkColumn(&fixedBulkSession{
		packet:  &gosnmp.SnmpPacket{Error: gosnmp.GenErr},
		version: gosnmp.Version2c,
	}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.Error(t, res.err)
	assert.Contains(t, res.err.Error(), "error-status")
}

func TestWalkSNMPv1NoSuchNameCompletes(t *testing.T) {
	res := walkColumn(&fixedBulkSession{
		packet:  &gosnmp.SnmpPacket{Error: gosnmp.NoSuchName},
		version: gosnmp.Version1,
	}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.NoError(t, res.err)
	assert.Empty(t, res.values)
}

func TestWalkUndecodableOnlyIsError(t *testing.T) {
	prefix := oidDot1qTpFdbPort
	res := walkColumn(&sequencedBulkSession{
		version: gosnmp.Version2c,
		packets: []*gosnmp.SnmpPacket{{
			Variables: []gosnmp.SnmpPDU{
				{Name: prefix + ".1.1", Type: gosnmp.Null, Value: nil},
				{Name: prefix + ".end", Type: gosnmp.EndOfMibView, Value: nil},
			},
		}},
	}, prefix, 10, 100, time.Time{})
	assert.ErrorIs(t, res.err, errWalkUndecodable)
	assert.Empty(t, res.values)
}

func TestWalkStopsWhenPageLeavesSubtree(t *testing.T) {
	oid := oidDot1qTpFdbPort + ".1.10.20.30.40.50.60"
	res := walkColumn(&sequencedBulkSession{
		version: gosnmp.Version2c,
		packets: []*gosnmp.SnmpPacket{{
			Variables: []gosnmp.SnmpPDU{
				{Name: oid, Type: gosnmp.Integer, Value: 1},
				{Name: oid + ".end", Type: gosnmp.EndOfMibView, Value: nil},
			},
		}},
	}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.NoError(t, res.err)
	require.Len(t, res.values, 1)
}

func TestWalkEmptyFirstPacketIsError(t *testing.T) {
	res := walkColumn(&fixedBulkSession{
		packet:  &gosnmp.SnmpPacket{Variables: nil},
		version: gosnmp.Version2c,
	}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.Error(t, res.err)
	assert.Contains(t, res.err.Error(), "did not advance")
	assert.Empty(t, res.values)
}

func TestWalkEmptyPacketAfterRowsIsError(t *testing.T) {
	oid := oidDot1qTpFdbPort + ".1.10.20.30.40.50.60"
	res := walkColumn(&sequencedBulkSession{
		version: gosnmp.Version2c,
		packets: []*gosnmp.SnmpPacket{
			{Variables: []gosnmp.SnmpPDU{{Name: oid, Type: gosnmp.Integer, Value: 1}}},
			{Variables: nil},
		},
	}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.Error(t, res.err)
	assert.Contains(t, res.err.Error(), "did not advance")
}

func TestWalkRepeatingOIDIsError(t *testing.T) {
	oid := oidDot1qTpFdbPort + ".1.10.20.30.40.50.60"
	packet := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{
		Name:  oid,
		Type:  gosnmp.Integer,
		Value: 1,
	}}}
	res := walkColumn(&fixedBulkSession{packet: packet, version: gosnmp.Version2c}, oidDot1qTpFdbPort, 10, 100, time.Time{})
	assert.Error(t, res.err)
	assert.Empty(t, res.reason)
	assert.Contains(t, res.err.Error(), "did not advance")
}

func TestWalkDeadline(t *testing.T) {
	sess := session.CreateFakeSession()
	sess.SetInt("1.3.6.1.2.1.17.7.1.2.2.1.2.1.10.20.30.40.50.60", 1)
	res := walkColumn(sess, oidDot1qTpFdbPort, 10, 100, time.Now().Add(-time.Second))
	assert.Equal(t, reasonMaxDuration, res.reason)
	assert.Error(t, res.err)
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// incompleteThenFail returns one in-table page without EndOfMibView, then errors.
type incompleteThenFail struct {
	*session.FakeSession
	prefix string
	first  *gosnmp.SnmpPacket
	seen   bool
}

func (s *incompleteThenFail) GetBulk(oids []string, bulkMaxRepetitions uint32) (*gosnmp.SnmpPacket, error) {
	if matchesOIDPrefix(oids, s.prefix) {
		if s.seen {
			return nil, errors.New("simulated timeout")
		}
		s.seen = true
		return s.first, nil
	}
	return s.FakeSession.GetBulk(oids, bulkMaxRepetitions)
}

type failFirstPrefix struct {
	*session.FakeSession
	prefix string
}

func (s *failFirstPrefix) GetBulk(oids []string, bulkMaxRepetitions uint32) (*gosnmp.SnmpPacket, error) {
	if matchesOIDPrefix(oids, s.prefix) {
		return nil, errors.New("unsupported")
	}
	return s.FakeSession.GetBulk(oids, bulkMaxRepetitions)
}

func matchesOIDPrefix(oids []string, prefix string) bool {
	return len(oids) == 1 && strings.HasPrefix(strings.TrimLeft(oids[0], "."), prefix)
}

type sequencedBulkSession struct {
	packets []*gosnmp.SnmpPacket
	i       int
	version gosnmp.SnmpVersion
}

func (s *sequencedBulkSession) Connect() error { return nil }
func (s *sequencedBulkSession) Close() error   { return nil }
func (s *sequencedBulkSession) Get([]string) (*gosnmp.SnmpPacket, error) {
	return s.nextPacket()
}
func (s *sequencedBulkSession) GetBulk([]string, uint32) (*gosnmp.SnmpPacket, error) {
	return s.nextPacket()
}
func (s *sequencedBulkSession) GetNext([]string) (*gosnmp.SnmpPacket, error) {
	return s.nextPacket()
}
func (s *sequencedBulkSession) nextPacket() (*gosnmp.SnmpPacket, error) {
	if s.i >= len(s.packets) {
		return nil, errors.New("unexpected extra request")
	}
	p := s.packets[s.i]
	s.i++
	return p, nil
}
func (s *sequencedBulkSession) GetSnmpGetCount() uint32        { return 0 }
func (s *sequencedBulkSession) GetSnmpGetBulkCount() uint32    { return 0 }
func (s *sequencedBulkSession) GetSnmpGetNextCount() uint32    { return 0 }
func (s *sequencedBulkSession) GetVersion() gosnmp.SnmpVersion { return s.version }
func (s *sequencedBulkSession) IsUnconnectedUDP() bool         { return false }

type fixedBulkSession struct {
	packet  *gosnmp.SnmpPacket
	version gosnmp.SnmpVersion
}

func (s *fixedBulkSession) Connect() error { return nil }
func (s *fixedBulkSession) Close() error   { return nil }
func (s *fixedBulkSession) Get([]string) (*gosnmp.SnmpPacket, error) {
	return s.packet, nil
}
func (s *fixedBulkSession) GetBulk([]string, uint32) (*gosnmp.SnmpPacket, error) {
	return s.packet, nil
}
func (s *fixedBulkSession) GetNext([]string) (*gosnmp.SnmpPacket, error) {
	return s.packet, nil
}
func (s *fixedBulkSession) GetSnmpGetCount() uint32     { return 0 }
func (s *fixedBulkSession) GetSnmpGetBulkCount() uint32 { return 0 }
func (s *fixedBulkSession) GetSnmpGetNextCount() uint32 { return 0 }
func (s *fixedBulkSession) GetVersion() gosnmp.SnmpVersion {
	return s.version
}
func (s *fixedBulkSession) IsUnconnectedUDP() bool { return false }
