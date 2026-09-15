// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fdb collects bounded SNMP forwarding-database snapshots.
package fdb

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Config is the collection limits for one FDB attempt.
type Config struct {
	DeviceID           string
	MaxEntries         int
	MaxDuration        time.Duration
	BulkMaxRepetitions uint32
}

// Result is one FDB collection attempt. Entries are set only on a complete snapshot.
type Result struct {
	Status  metadata.FDBStatusMetadata
	Entries []metadata.FDBEntryMetadata
}

type tableSpec struct {
	portOID   string
	statusOID string
	source    string
	qbridge   bool
}

// Collect walks Q-BRIDGE then BRIDGE, resolves bridge port to ifIndex, and
// returns either a complete snapshot or status-only on truncation/error.
func Collect(sess session.Session, cfg Config) Result {
	start := time.Now()
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 10000
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = 10 * time.Second
	}
	if cfg.BulkMaxRepetitions == 0 {
		cfg.BulkMaxRepetitions = 10
	}
	deadline := start.Add(cfg.MaxDuration)

	status := metadata.FDBStatusMetadata{
		DeviceID: cfg.DeviceID,
		Status:   metadata.FDBCollectStatusSuccess,
	}

	portMap := walkPortIfIndex(sess, cfg, deadline)

	qbridge := tableSpec{portOID: oidDot1qTpFdbPort, statusOID: oidDot1qTpFdbStatus, source: metadata.FDBSourceQBridge, qbridge: true}
	entries, reason, err := collectTable(sess, cfg, deadline, portMap, qbridge)
	if reason != "" {
		return truncatedResult(cfg.DeviceID, start, reason)
	}
	if err == nil && len(entries) > 0 {
		return successResult(cfg.DeviceID, start, metadata.FDBSourceQBridge, entries)
	}

	bridge := tableSpec{portOID: oidDot1dTpFdbPort, statusOID: oidDot1dTpFdbStatus, source: metadata.FDBSourceBridge, qbridge: false}
	entries, reason, err = collectTable(sess, cfg, deadline, portMap, bridge)
	if reason != "" {
		return truncatedResult(cfg.DeviceID, start, reason)
	}
	if err == nil && len(entries) > 0 {
		return successResult(cfg.DeviceID, start, metadata.FDBSourceBridge, entries)
	}
	if err != nil {
		status.Status = metadata.FDBCollectStatusError
		status.Reason = err.Error()
		status.DurationMs = time.Since(start).Milliseconds()
		return Result{Status: status}
	}

	status.DurationMs = time.Since(start).Milliseconds()
	return Result{Status: status}
}

func walkPortIfIndex(sess session.Session, cfg Config, deadline time.Time) map[int32]int32 {
	res := walkColumn(sess, oidDot1dBasePortIfIndex, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	out := make(map[int32]int32, len(res.values))
	for index, value := range res.values {
		port, ok := int32Value(index)
		if !ok || port <= 0 {
			continue
		}
		ifIndex, ok := resultInt32(value)
		if !ok || ifIndex <= 0 {
			continue
		}
		out[port] = ifIndex
	}
	return out
}

func collectTable(sess session.Session, cfg Config, deadline time.Time, portMap map[int32]int32, spec tableSpec) ([]metadata.FDBEntryMetadata, string, error) {
	ports := walkColumn(sess, spec.portOID, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	if ports.reason != "" {
		return nil, ports.reason, ports.err
	}
	if ports.err != nil {
		return nil, "", ports.err
	}
	if len(ports.values) == 0 {
		return nil, "", nil
	}

	statuses := walkColumn(sess, spec.statusOID, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	if statuses.reason != "" || statuses.err != nil {
		// Status is advisory; keep port rows when status is missing or incomplete.
		log.Debugf("fdb status walk incomplete for %s: reason=%s err=%v", spec.source, statuses.reason, statuses.err)
		statuses.values = nil
	}

	var entries []metadata.FDBEntryMetadata
	for index, portVal := range ports.values {
		entry, ok := buildEntry(cfg.DeviceID, spec, index, portVal, statuses.values[index], portMap)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, "", nil
}

func buildEntry(deviceID string, spec tableSpec, index string, portVal valuestore.ResultValue, statusVal valuestore.ResultValue, portMap map[int32]int32) (metadata.FDBEntryMetadata, bool) {
	var fdbID, mac string
	var ok bool
	if spec.qbridge {
		fdbID, mac, ok = parseQBridgeIndex(index)
	} else {
		mac, ok = parseBridgeIndex(index)
	}
	if !ok || isZeroMAC(mac) || isBroadcastMAC(mac) || isMulticastMAC(mac) {
		return metadata.FDBEntryMetadata{}, false
	}
	port, ok := resultInt32(portVal)
	if !ok || port <= 0 {
		return metadata.FDBEntryMetadata{}, false
	}
	if statusVal.Value != nil {
		status, statusOK := resultInt32(statusVal)
		if statusOK && status != fdbStatusLearned {
			return metadata.FDBEntryMetadata{}, false
		}
	}

	entry := metadata.FDBEntryMetadata{
		DeviceID:   deviceID,
		FDBID:      fdbID,
		MacAddress: mac,
		BridgePort: port,
		Source:     spec.source,
	}
	if ifIndex, mapped := portMap[port]; mapped {
		entry.InterfaceIndex = ifIndex
		entry.InterfaceID = deviceID + ":" + strconv.Itoa(int(ifIndex))
	}
	return entry, true
}

func successResult(deviceID string, start time.Time, source string, entries []metadata.FDBEntryMetadata) Result {
	return Result{
		Status: metadata.FDBStatusMetadata{
			DeviceID:   deviceID,
			Status:     metadata.FDBCollectStatusSuccess,
			Source:     source,
			RowCount:   len(entries),
			DurationMs: time.Since(start).Milliseconds(),
		},
		Entries: entries,
	}
}

func truncatedResult(deviceID string, start time.Time, reason string) Result {
	return Result{
		Status: metadata.FDBStatusMetadata{
			DeviceID:   deviceID,
			Status:     metadata.FDBCollectStatusTruncated,
			RowCount:   0,
			DurationMs: time.Since(start).Milliseconds(),
			Reason:     reason,
		},
	}
}

func resultInt32(value valuestore.ResultValue) (int32, bool) {
	f, err := value.ToFloat64()
	if err != nil {
		return 0, false
	}
	return int32(f), true
}

func int32Value(s string) (int32, bool) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return int32(n), true
}
