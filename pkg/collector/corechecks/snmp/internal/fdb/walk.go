// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fdb

import (
	"errors"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const (
	reasonMaxEntries  = "max_entries"
	reasonMaxDuration = "max_duration"
)

var (
	errWalkMaxDuration = errors.New("fdb walk exceeded max duration")
	errWalkMaxEntries  = errors.New("fdb walk exceeded max entries")
)

type walkResult struct {
	values map[string]valuestore.ResultValue
	reason string
	err    error
}

func walkColumn(sess session.Session, columnOID string, bulkMaxRepetitions uint32, maxRows int, deadline time.Time) walkResult {
	values := make(map[string]valuestore.ResultValue)
	curOID := columnOID
	seen := make(map[string]struct{})
	useGetNext := sess.GetVersion() == gosnmp.Version1
	prefix := columnOID + "."

	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return walkResult{values: values, reason: reasonMaxDuration, err: errWalkMaxDuration}
		}

		packet, err := nextPacket(sess, curOID, bulkMaxRepetitions, useGetNext, maxRows-len(values))
		if err != nil {
			return walkResult{values: values, err: err}
		}

		inTable := 0
		lastOID := curOID
		for _, pdu := range packet.Variables {
			oid := strings.TrimLeft(pdu.Name, ".")
			if pdu.Type == gosnmp.EndOfContents || pdu.Type == gosnmp.EndOfMibView || pdu.Type == gosnmp.NoSuchInstance || pdu.Type == gosnmp.NoSuchObject {
				continue
			}
			if !strings.HasPrefix(oid, prefix) {
				continue
			}
			if _, ok := seen[oid]; ok {
				continue
			}
			seen[oid] = struct{}{}
			inTable++
			_, value, err := valuestore.GetResultValueFromPDU(pdu)
			if err != nil {
				continue
			}
			index := oid[len(prefix):]
			values[index] = value
			lastOID = oid
			if len(values) > maxRows {
				return walkResult{values: values, reason: reasonMaxEntries, err: errWalkMaxEntries}
			}
		}
		if inTable == 0 {
			return walkResult{values: values}
		}
		if lastOID == curOID {
			return walkResult{values: values}
		}
		curOID = lastOID
	}
}

func nextPacket(sess session.Session, curOID string, bulkMaxRepetitions uint32, useGetNext bool, remaining int) (*gosnmp.SnmpPacket, error) {
	if useGetNext {
		return sess.GetNext([]string{curOID})
	}
	rep := bulkMaxRepetitions
	if remaining >= 0 {
		// Request one extra row so an exact maxRows table can finish cleanly
		// while a larger table is detected as truncated.
		need := uint32(remaining + 1)
		if need < rep {
			rep = need
		}
	}
	if rep < 1 {
		rep = 1
	}
	return sess.GetBulk([]string{curOID}, rep)
}
