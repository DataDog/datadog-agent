// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build test

package listeners

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

// numbersToOID converts a list of numbers back to a dotted OID string.
func numbersToOID(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// fakeSession implements snmpSession with in-memory PDU data.
// Missing OIDs return NoSuchObject; GetNext returns EndOfMibView past the end.
type fakeSession struct {
	connectErr error
	data       map[string]gosnmp.SnmpPDU
}

// createFakeSession creates a new fakeSession with an empty set of data.
func createFakeSession() *fakeSession {
	return &fakeSession{data: make(map[string]gosnmp.SnmpPDU)}
}

// Set adds a PDU with the given attributes, replacing any with the same OID.
func (fs *fakeSession) Set(oid string, typ gosnmp.Asn1BER, val any) *fakeSession {
	fs.data[oid] = gosnmp.SnmpPDU{Name: oid, Type: typ, Value: val}
	return fs
}

// SetStr adds an OctetString PDU with the given OID and value.
func (fs *fakeSession) SetStr(oid string, value string) *fakeSession {
	return fs.Set(oid, gosnmp.OctetString, []byte(value))
}

// SetObj adds an ObjectIdentifier PDU with the given OID and value.
func (fs *fakeSession) SetObj(oid string, value string) *fakeSession {
	return fs.Set(oid, gosnmp.ObjectIdentifier, value)
}

// SetTime adds a TimeTicks PDU with the given OID and value.
func (fs *fakeSession) SetTime(oid string, ticks uint32) *fakeSession {
	return fs.Set(oid, gosnmp.TimeTicks, ticks)
}

// sortedOIDs returns all OIDs in data sorted lexicographically.
func (fs *fakeSession) sortedOIDs() [][]int {
	oids := make([][]int, 0, len(fs.data))
	for oid := range fs.data {
		nums, err := gosnmplib.OIDToInts(oid)
		if err != nil {
			continue
		}
		oids = append(oids, nums)
	}
	sort.Slice(oids, func(i, j int) bool {
		return gosnmplib.CmpOIDs(oids[i], oids[j]).IsBefore()
	})
	return oids
}

// Connect returns connectErr if set, nil otherwise.
func (fs *fakeSession) Connect() error { return fs.connectErr }

// Close is a no-op.
func (fs *fakeSession) Close() error { return nil }

// Get returns the PDUs for the requested OIDs, or NoSuchObject for missing ones.
func (fs *fakeSession) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	vars := make([]gosnmp.SnmpPDU, len(oids))
	for i, oid := range oids {
		v, ok := fs.data[oid]
		if !ok {
			v = gosnmp.SnmpPDU{Name: oid, Type: gosnmp.NoSuchObject, Value: nil}
		}
		vars[i] = v
	}
	return &gosnmp.SnmpPacket{Variables: vars}, nil
}

// GetNext returns the first PDU after each requested OID, or EndOfMibView past the end.
func (fs *fakeSession) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	knownOIDs := fs.sortedOIDs()
	vars := make([]gosnmp.SnmpPDU, len(oids))
	for i, oid := range oids {
		nums, err := gosnmplib.OIDToInts(oid)
		if err != nil {
			vars[i] = gosnmp.SnmpPDU{Name: oid, Type: gosnmp.EndOfMibView, Value: nil}
			continue
		}
		idx := sort.Search(len(knownOIDs), func(j int) bool {
			return gosnmplib.CmpOIDs(nums, knownOIDs[j]).IsBefore()
		})
		if idx >= len(knownOIDs) {
			vars[i] = gosnmp.SnmpPDU{Name: oid, Type: gosnmp.EndOfMibView, Value: nil}
		} else {
			vars[i] = fs.data[numbersToOID(knownOIDs[idx])]
		}
	}
	return &gosnmp.SnmpPacket{Variables: vars}, nil
}

// errorSession implements snmpSession returning configurable errors for each operation.
type errorSession struct {
	connectErr error
	getNextErr error
	getErr     error
	getNextPkt *gosnmp.SnmpPacket
	getPkt     *gosnmp.SnmpPacket
}

func (es *errorSession) Connect() error                           { return es.connectErr }
func (es *errorSession) Close() error                             { return nil }
func (es *errorSession) Get([]string) (*gosnmp.SnmpPacket, error) { return es.getPkt, es.getErr }
func (es *errorSession) GetNext([]string) (*gosnmp.SnmpPacket, error) {
	return es.getNextPkt, es.getNextErr
}

// testFactoryCall records a single invocation of the test session factory.
type testFactoryCall struct {
	Auth     snmp.Authentication
	DeviceIP string
	Port     uint16
}

// testSessionFactory maps device IPs (with optional auth) to fake sessions.
// Unregistered devices default to returning a connection error.
type testSessionFactory struct {
	mu       sync.Mutex
	sessions map[string]snmpSession
	calls    []testFactoryCall
}

// newTestSessionFactory creates a factory with no registered sessions.
func newTestSessionFactory() *testSessionFactory {
	return &testSessionFactory{sessions: make(map[string]snmpSession)}
}

// build looks up a session by "IP:community", "IP:user", or "IP" in that order.
func (f *testSessionFactory) build(auth snmp.Authentication, deviceIP string, port uint16) (snmpSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFactoryCall{Auth: auth, DeviceIP: deviceIP, Port: port})

	for _, key := range []string{
		deviceIP + ":" + auth.Community,
		deviceIP + ":" + auth.User,
		deviceIP,
	} {
		if sess, ok := f.sessions[key]; ok {
			return sess, nil
		}
	}
	return &errorSession{connectErr: errors.New("connection refused")}, nil
}

// makeReachableSession creates a fakeSession that passes the reachability check.
func makeReachableSession() *fakeSession {
	return createFakeSession().SetStr("1.0.0.1", "reachable")
}

// makeReachableSessionWithDeviceInfo creates a fakeSession that passes reachability
// and returns device info for deduplication.
func makeReachableSessionWithDeviceInfo(sysName, sysDescr, sysObjectID string, sysUptime uint32) *fakeSession {
	return makeReachableSession().
		SetStr(snmp.DeviceSysNameOid, sysName).
		SetStr(snmp.DeviceSysDescrOid, sysDescr).
		SetTime(snmp.DeviceSysUptimeOid, sysUptime).
		SetObj(snmp.DeviceSysObjectIDOid, sysObjectID)
}
