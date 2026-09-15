// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fdb collects bounded SNMP forwarding-database observations.
package fdb

import (
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

const (
	// CollectionInterval is the fixed interval between FDB collection attempts.
	CollectionInterval = 5 * time.Minute
	maxEntries         = 2000
	maxDuration        = 10 * time.Second

	// SourceQBridge identifies observations from Q-BRIDGE-MIB.
	SourceQBridge = "q-bridge"
	// SourceBridge identifies observations from BRIDGE-MIB.
	SourceBridge = "bridge"
)

// Outcome is the internal result of an FDB collection attempt.
type Outcome string

const (
	// OutcomeSuccess means collection completed without hitting a safety limit.
	OutcomeSuccess Outcome = "success"
	// OutcomeTruncated means a safety limit was hit and all rows were discarded.
	OutcomeTruncated Outcome = "truncated"
	// OutcomeError means collection failed and no rows were emitted.
	OutcomeError Outcome = "error"
)

type config struct {
	DeviceID           string
	MaxEntries         int
	MaxDuration        time.Duration
	BulkMaxRepetitions uint32
}

// Result is one FDB collection attempt. Entries are set only when collection completes.
type Result struct {
	Outcome  Outcome
	Source   string
	Reason   string
	Duration time.Duration
	Entries  []metadata.FDBEntryMetadata
}

type tableSpec struct {
	portOID   string
	statusOID string
	source    string
	qbridge   bool
}

// Collect walks Q-BRIDGE then BRIDGE and resolves bridge ports to ifIndex.
func Collect(sess session.Session, deviceID string, bulkMaxRepetitions uint32) Result {
	return collect(sess, config{
		DeviceID:           deviceID,
		MaxEntries:         maxEntries,
		MaxDuration:        maxDuration,
		BulkMaxRepetitions: bulkMaxRepetitions,
	})
}

func collect(sess session.Session, cfg config) Result {
	start := time.Now()
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = maxEntries
	}
	if cfg.MaxDuration <= 0 {
		cfg.MaxDuration = maxDuration
	}
	if cfg.BulkMaxRepetitions == 0 {
		cfg.BulkMaxRepetitions = 10
	}
	deadline := start.Add(cfg.MaxDuration)

	portMapResult := walkColumn(sess, oidDot1dBasePortIfIndex, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	if portMapResult.reason != "" {
		return truncatedResult(start, portMapResult.reason)
	}
	if portMapResult.err != nil {
		return errorResult(start, portMapResult.err)
	}
	portMap := buildPortIfIndexMap(portMapResult.values)

	qbridge := tableSpec{portOID: oidDot1qTpFdbPort, statusOID: oidDot1qTpFdbStatus, source: SourceQBridge, qbridge: true}
	entries, reason, err, started := collectTable(sess, cfg, deadline, portMap, qbridge)
	if reason != "" {
		return truncatedResult(start, reason)
	}
	if err != nil && started {
		return errorResult(start, err)
	}
	if err == nil && len(entries) > 0 {
		return successResult(start, SourceQBridge, entries)
	}

	bridge := tableSpec{portOID: oidDot1dTpFdbPort, statusOID: oidDot1dTpFdbStatus, source: SourceBridge, qbridge: false}
	entries, reason, err, _ = collectTable(sess, cfg, deadline, portMap, bridge)
	if reason != "" {
		return truncatedResult(start, reason)
	}
	if err == nil && len(entries) > 0 {
		return successResult(start, SourceBridge, entries)
	}
	if err != nil {
		return errorResult(start, err)
	}

	return successResult(start, "", nil)
}

func buildPortIfIndexMap(values map[string]valuestore.ResultValue) map[int32]int32 {
	out := make(map[int32]int32, len(values))
	for index, value := range values {
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

func collectTable(sess session.Session, cfg config, deadline time.Time, portMap map[int32]int32, spec tableSpec) ([]metadata.FDBEntryMetadata, string, error, bool) {
	ports := walkColumn(sess, spec.portOID, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	if ports.reason != "" {
		return nil, ports.reason, ports.err, len(ports.values) > 0
	}
	if ports.err != nil {
		return nil, "", ports.err, len(ports.values) > 0
	}
	if len(ports.values) == 0 {
		return nil, "", nil, false
	}

	statuses, reason, err := walkStatus(sess, spec.statusOID, cfg, deadline)
	if reason != "" || err != nil {
		return nil, reason, err, true
	}

	var entries []metadata.FDBEntryMetadata
	for index, portVal := range ports.values {
		if !learnedStatus(statuses, index) {
			continue
		}
		entry, ok := buildEntry(cfg.DeviceID, spec, index, portVal, portMap)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, "", nil, true
}

func walkStatus(sess session.Session, statusOID string, cfg config, deadline time.Time) (map[string]valuestore.ResultValue, string, error) {
	if statusOID == "" {
		return nil, "", nil
	}
	statuses := walkColumn(sess, statusOID, cfg.BulkMaxRepetitions, cfg.MaxEntries, deadline)
	if statuses.reason != "" {
		return nil, statuses.reason, statuses.err
	}
	if statuses.err != nil {
		return nil, "", statuses.err
	}
	return statuses.values, "", nil
}

func learnedStatus(statuses map[string]valuestore.ResultValue, index string) bool {
	if len(statuses) == 0 {
		return true
	}
	value, ok := statuses[index]
	if !ok {
		return false
	}
	status, ok := resultInt32(value)
	return ok && status == fdbStatusLearned
}

func buildEntry(deviceID string, spec tableSpec, index string, portVal valuestore.ResultValue, portMap map[int32]int32) (metadata.FDBEntryMetadata, bool) {
	var mac string
	var ok bool
	if spec.qbridge {
		mac, ok = parseQBridgeIndex(index)
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
	ifIndex, mapped := portMap[port]
	if !mapped {
		return metadata.FDBEntryMetadata{}, false
	}
	return metadata.FDBEntryMetadata{
		DeviceID:       deviceID,
		MacAddress:     mac,
		InterfaceIndex: ifIndex,
	}, true
}

func successResult(start time.Time, source string, entries []metadata.FDBEntryMetadata) Result {
	return Result{
		Outcome:  OutcomeSuccess,
		Source:   source,
		Duration: time.Since(start),
		Entries:  entries,
	}
}

func truncatedResult(start time.Time, reason string) Result {
	return Result{
		Outcome:  OutcomeTruncated,
		Reason:   reason,
		Duration: time.Since(start),
	}
}

func errorResult(start time.Time, err error) Result {
	return Result{
		Outcome:  OutcomeError,
		Reason:   err.Error(),
		Duration: time.Since(start),
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
