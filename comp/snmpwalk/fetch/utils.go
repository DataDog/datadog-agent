// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
	"strings"
)

// TODO: AVOID DUPLICATION WITH GetNextColumnOidNaive in snmp corecheck

// GetNextColumnOidNaive will return the next column OID for a given OID
// This is a naive algorithm based on detecting last `.1.` (expected to be table entry), increase the next digit and returns it.
// Caveat: if the input OID index contain `.1.` the function won't return the next column OID but all the OID elements until `.1.`.
func GetNextColumnOidNaive(oid string) string {
	oid = strings.TrimRight(strings.TrimLeft(oid, "."), ".")

	if !strings.HasPrefix(oid, "1.0.") && strings.Contains(oid, ".0.") {
		zeroIndex := strings.Index(oid, ".0.")
		prefix := oid[0:zeroIndex]

		segments := strings.Split(prefix, ".")
		lastDigit := segments[len(segments)-1]
		newPrefix := strings.Join(segments[0:len(segments)-1], ".")
		lastDigitNum, err := strconv.Atoi(lastDigit)
		if err != nil {
			log.Debugf("index is expected to be a integer, but it was not: %s", oid)
			return oid
		}
		oid = newPrefix + "." + strconv.Itoa(lastDigitNum+1)
	}

	idx := strings.LastIndex(oid, ".1.") // Try to find the table Entry OID
	if idx == -1 {
		// not found
		//return "", fmt.Errorf("the oid is not a column oid: %s", oid)
		return oid
	}
	tableOid := oid[0:idx]
	rowFullIndex := oid[idx+3:] // +3 to skip `.1.`
	if !strings.Contains(rowFullIndex, ".") {
		idx = strings.LastIndex(tableOid+".", ".1.") // Try to find the table Entry OID
		if idx == -1 {
			// not found
			return oid
		}
		tableOid = oid[0:idx]
		rowFullIndex = oid[idx+3:] // +3 to skip `.1.`
	}
	rowFirstIndex := strings.Split(rowFullIndex, ".")[0]
	rowFirstIndexNum, err := strconv.Atoi(rowFirstIndex)
	if err != nil {
		return oid
	}
	// `1` is the table entry oid, it's always `1`
	return tableOid + ".1." + strconv.Itoa(rowFirstIndexNum+1)
}
