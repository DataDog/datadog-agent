// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

//nolint:revive // TODO(NDM) Fix revive linter
package session

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// oidToNumbers parses an OID into a list of numbers.
// It is the inverse of numbersToOID.
func oidToNumbers(oid string) ([]int, error) {
	oid = strings.TrimLeft(oid, ".")
	strNumbers := strings.Split(oid, ".")
	var numbers []int
	for _, strNumber := range strNumbers {
		num, err := strconv.Atoi(strNumber)
		if err != nil {
			return nil, fmt.Errorf("error converting digit %s (oid=%s)", strNumber, oid)
		}
		numbers = append(numbers, num)
	}
	return numbers, nil
}

// numbersToOID converts a list of numbers back to an OID.
// It is the inverse of oidToNumbers.
func numbersToOID(nums []int) string {
	segments := make([]string, len(nums))
	for i, k := range nums {
		segments[i] = fmt.Sprint(k)
	}
	return strings.Join(segments, ".")
}

// cmpOIDs return -1 if a < b, 1 if a > b, and 0 otherwise.
// Ordering is lexicographic by OID.
func cmpOIDs(a, b []int) int {
	for i := range a {
		if i >= len(b) {
			return 1
		}
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	if len(b) > len(a) {
		return -1
	}
	return 0
}

// FakeSession implements Session wrapping around a fixed set of PDUs.
// Caveats:
//   - Fetching an object that isn't there will always return NoSuchObject,
//     never NoSuchInstance. I don't think we can do NoSuchInstance without
//     parsing a MIB to know what paths could exist.
type FakeSession struct {
	data map[string]gosnmp.SnmpPDU
	// oids is a sorted slice of all OIDs in data, stored as []ints.
	oids [][]int
	// dirty indicates whether oids needs to be rebuilt.
	dirty bool
}

// CreateFakeSession creates a new FakeSession with an empty set of data.
func CreateFakeSession() *FakeSession {
	return &FakeSession{
		data: make(map[string]gosnmp.SnmpPDU),
	}
}

// Set creates a new PDU with the given attributes, replacing any in the
// session with the same OID.
func (fs *FakeSession) Set(oid string, typ gosnmp.Asn1BER, val any) *FakeSession {
	if _, ok := fs.data[oid]; !ok {
		// new OID, need to rebuild oids
		fs.dirty = true
	}
	fs.data[oid] = gosnmp.SnmpPDU{
		Name:  oid,
		Type:  typ,
		Value: val,
	}
	return fs
}

// SetMany adds many PDUs to the session at once.
func (fs *FakeSession) SetMany(pdus ...gosnmp.SnmpPDU) {
	fs.dirty = true
	for _, pdu := range pdus {
		fs.data[pdu.Name] = pdu
	}
}

// getOIDs returns a sorted list of all OIDs in fs.data.
func (fs *FakeSession) getOIDs() [][]int {
	if fs.dirty {
		fs.oids = [][]int{}
		for oid := range fs.data {
			nums, err := oidToNumbers(oid)
			if err != nil {
				continue
			}
			fs.oids = append(fs.oids, nums)
		}
		sort.Slice(fs.oids, func(i, j int) bool {
			return cmpOIDs(fs.oids[i], fs.oids[j]) < 0
		})
		fs.dirty = false
	}
	return fs.oids
}

// Connect is a no-op.
func (fs *FakeSession) Connect() error {
	return nil
}

// Close is a no-op.
func (fs *FakeSession) Close() error {
	return nil
}

// GetVersion always returns 3.
func (fs *FakeSession) GetVersion() gosnmp.SnmpVersion {
	return gosnmp.Version3
}

// Get gets the values for the given OIDs. OIDs not in the session will return
// PDUs of type NoSuchObject.
func (fs *FakeSession) Get(oids []string) (result *gosnmp.SnmpPacket, err error) {
	vars := make([]gosnmp.SnmpPDU, len(oids))
	for i, oid := range oids {
		v, ok := fs.data[oid]
		if !ok {
			v = gosnmp.SnmpPDU{
				Name:  oid,
				Type:  gosnmp.NoSuchObject,
				Value: nil,
			}
		}
		vars[i] = v
	}
	return &gosnmp.SnmpPacket{
		Variables: vars,
	}, nil
}

// nextIndex finds the index of the first OID greater than the input.
// If oid is later than all known OIDs, it returns len(fs.OIDs).
func (fs *FakeSession) nextIndex(oid []int) int {
	oids := fs.getOIDs()
	return sort.Search(len(oids), func(i int) bool {
		return cmpOIDs(oid, oids[i]) < 0
	})
}

// getNexts returns the items expected by a GetBulk request.
func (fs *FakeSession) getNexts(oids []string, count int) ([]gosnmp.SnmpPDU, error) {
	knownOIDs := fs.getOIDs()
	vars := make([]gosnmp.SnmpPDU, len(oids)*count)
	for i, oid := range oids {
		index := len(knownOIDs)
		nums, err := oidToNumbers(oid)
		if err == nil {
			index = fs.nextIndex(nums)
		}
		for offset := 0; offset < count; offset++ {
			var v gosnmp.SnmpPDU
			if index+offset >= len(knownOIDs) {
				// out of bounds
				v = gosnmp.SnmpPDU{
					Name:  oid,
					Type:  gosnmp.EndOfMibView,
					Value: nil,
				}
			} else {
				v = fs.data[numbersToOID(knownOIDs[index+offset])]
			}
			vars[offset*len(oids)+i] = v
		}
	}
	return vars, nil
}

// GetBulk returns the `count` next PDUs after each of the given oids.
// If it runs off the end of the data the extra values will all be EndOfMibView
// PDUs.
func (fs *FakeSession) GetBulk(oids []string, count uint32) (*gosnmp.SnmpPacket, error) {
	vars, err := fs.getNexts(oids, int(count))
	if err != nil {
		return nil, err
	}
	return &gosnmp.SnmpPacket{
		Variables: vars,
	}, nil
}

// GetNext returns the first PDU after each of the given OIDs. An OID with
// nothing greater than it will result in an EndOfMibView PDU.
func (fs *FakeSession) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	vars, err := fs.getNexts(oids, 1)
	if err != nil {
		return nil, err
	}
	return &gosnmp.SnmpPacket{
		Variables: vars,
	}, nil
}

// SetByte adds an OctetString PDU with the given OID and value
func (fs *FakeSession) SetByte(oid string, value []byte) *FakeSession {
	return fs.Set(oid, gosnmp.OctetString, value)
}

// SetStr adds an OctetString PDU with the given OID and value
func (fs *FakeSession) SetStr(oid string, value string) *FakeSession {
	return fs.SetByte(oid, []byte(value))
}

// SetObj adds an ObjectIdentifier PDU with the given OID and value
func (fs *FakeSession) SetObj(oid string, value string) *FakeSession {
	return fs.Set(oid, gosnmp.ObjectIdentifier, value)
}

// SetTime adds a TimeTicks PDU with the given OID and value
func (fs *FakeSession) SetTime(oid string, ticks int) *FakeSession {
	return fs.Set(oid, gosnmp.TimeTicks, ticks)
}

// SetInt adds an Integer PDU with the given OID and value
func (fs *FakeSession) SetInt(oid string, value int) *FakeSession {
	return fs.Set(oid, gosnmp.Integer, value)
}

// SetIP adds an IP PDU with the given OID and value
func (fs *FakeSession) SetIP(oid string, value string) *FakeSession {
	return fs.Set(oid, gosnmp.IPAddress, value)
}
