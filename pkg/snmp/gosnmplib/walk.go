// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib

import (
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	// Note that gosnmp.walk uses ".1.3.6.1.2.1" as its base ID, but we
	// sometimes want things like LLDP data that are under lower prefixes
	// (LLDP goes under .1.0.*). So we just start as low as possible.
	baseOID = ".0.0"
	// Java SNMP uses 50, snmp-net uses 10
	defaultMaxRepetitions = 50
)

// ConditionalWalk mimics gosnmp.GoSNMP.Walk, except that the walkFn can return
// a next OID to walk from. Use e.g. SkipOIDRowsNaive to skip over additional
// rows.
// This code is adapated directly from gosnmp's walk function.
func ConditionalWalk(session *gosnmp.GoSNMP, rootOID string, useBulk bool, walkFn func(dataUnit gosnmp.SnmpPDU) (string, error)) error {
	if rootOID == "" || rootOID == "." {
		rootOID = baseOID
	}

	if !strings.HasPrefix(rootOID, ".") {
		rootOID = string(".") + rootOID
	}

	oid := rootOID
	requests := 0
	maxReps := session.MaxRepetitions

	if maxReps == 0 {
		maxReps = defaultMaxRepetitions
	}

RequestLoop:
	for {
		requests++
		var response *gosnmp.SnmpPacket
		var err error
		if useBulk {
			response, err = session.GetBulk([]string{oid}, 0, maxReps)
		} else {
			response, err = session.GetNext([]string{oid})
		}
		if err != nil {
			return err
		}
		if len(response.Variables) == 0 {
			break RequestLoop
		}
		if response.Error != gosnmp.NoError {
			session.Logger.Printf("ConditionalWalk terminated with %s", response.Error.String())
			break RequestLoop
		}

		for i, pdu := range response.Variables {
			if pdu.Type == gosnmp.EndOfMibView || pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance {
				session.Logger.Printf("ConditionalWalk terminated with type 0x%x", pdu.Type)
				break RequestLoop
			}
			// skip PDUs that are less than our next OID when we're handling
			// multiple PDUs from one getBulk
			if i > 0 {
				next, err := OIDToInts(oid)
				if err != nil {
					return err
				}
				this, err := OIDToInts(pdu.Name)
				if err != nil {
					return err
				}
				// Skip this PDU if our next OID is after this one - it means we
				// skipped it because of walkFn
				if CmpOIDs(next, this).IsAfter() {
					continue
				}
			}
			// Report our pdu
			oid, err = walkFn(pdu)
			if err != nil {
				return err
			}
			if oid == "" {
				oid = pdu.Name
			}
		}
	}
	session.Logger.Printf("ConditionalWalk completed in %d requests", requests)
	return nil
}

// SkipOIDRowsNaive takes an OID and returns a suitable OID to pass to GetNext
// in order to fetch the next OID after the given one excluding additional table
// rows. If the OID is for a scalar value, it is returned unchanged; otherwise,
// it is the OID for a row in a table, and the returned OID will be just less
// than the first OID for the next column of the table.
//
// This is a naive, MIB-less algorithm based on the fact that if a table has OID
// X, then the TableEntry that describes the table type will have OID `X.1` and
// every column in the table will have an OID like `X.1.n`. Thus, if we grab the
// last segment other than the end that is `1` in the input OID, and increment
// the next segment by 1, there's a good chance we'll get the next row.
//
// Three caveats: first, if a table index ends in .0, this will mistake it for a
// scalar and return it. Second, if a table index contains '.1.', we will
// interpret that as the table index instead. Third, if the first row of a table
// has OID .1, then that will also be mistaken for the table entry. In all three
// of these cases, the result is that we end up grabbing additional rows of a
// table, but we will never skip over any rows or scalars, so it is safe to use
// this to determine a superset of all OIDs present on a device.
//
// It is unlikely we can do better than this without having relevant MIB data.
func SkipOIDRowsNaive(oid string) string {
	oid = strings.TrimRight(strings.TrimLeft(oid, "."), ".")
	if strings.HasSuffix(oid, ".0") { // Possibly a scalar OID
		return oid
	}
	idx := strings.LastIndex(oid, ".1.") // Try to find the table Entry OID
	if idx == -1 {                       // Not a table OID
		return oid
	}
	tableOid := oid[0:idx]
	rowFullIndex := oid[idx+3:] // +3 to skip `.1.`
	rowFirstIndex := strings.Split(rowFullIndex, ".")[0]
	rowFirstIndexNum, err := strconv.Atoi(rowFirstIndex)
	if err != nil { // This shouldn't be possible unless the OID is malformed
		return oid
	}
	// `1` is the table entry oid, it's always `1`
	return tableOid + ".1." + strconv.Itoa(rowFirstIndexNum+1)
}
