// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package session

import (
	"fmt"
	"strconv"
	"strings"
)

// GetNextColumnOidNaive will return the next column OID for a given OID
// This is a naive algorithm based on detecting `.1.` (the table Entry oid that contains), increase the next digit and returns it.
// Caveat: if the input OID index contain `.1.` the function won't return the next column OID but all the OID elements until `.1.`.
func GetNextColumnOidNaive(oid string) (string, error) {
	oid = strings.TrimRight(strings.TrimLeft(oid, "."), ".")
	idx := strings.LastIndex(oid, ".1.") // Try to find the table Entry OID
	if idx == -1 {
		// not found
		return "", fmt.Errorf("the oid is not a column oid: %s", oid)
	}
	tableOid := oid[0:idx]
	rowFullIndex := oid[idx+3:] // +3 to skip `.1.`
	rowFirstIndex := strings.Split(rowFullIndex, ".")[0]
	rowFirstIndexNum, err := strconv.Atoi(rowFirstIndex)
	if err != nil {
		return "", fmt.Errorf("index is expected to be a integer, but it was not: %s", oid)
	}
	// `1` is the table entry oid, it's always `1`
	return tableOid + ".1." + strconv.Itoa(rowFirstIndexNum+1), nil
}
