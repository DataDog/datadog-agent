package session

import (
	"fmt"
	"strconv"
	"strings"
)

func GetNextColumnOid(oid string) (string, error) {
	idx := strings.LastIndex(oid, ".1.")
	if idx == -1 { // not found
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
