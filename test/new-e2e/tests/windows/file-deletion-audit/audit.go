// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package filedeletionaudit contains host-independent decoding and
// classification logic for Windows file-deletion audit events.
package filedeletionaudit

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	pathpkg "path"
	"strconv"
	"strings"
	"time"
)

const (
	deleteAccess          = 0x00010000
	fileDeleteChildAccess = 0x00000040
)

// SecurityLogCheckpoint identifies a point in the Security log and records
// enough log metadata to diagnose rollover and capacity issues.
type SecurityLogCheckpoint struct {
	RecordID           uint64 `json:"RecordID"`
	OldestRecordID     uint64 `json:"OldestRecordID"`
	RecordCount        uint64 `json:"RecordCount"`
	FileSize           uint64 `json:"FileSize"`
	MaximumSizeInBytes uint64 `json:"MaximumSizeInBytes"`
}

// FileDeletionEvent is the locale-independent subset of Security event 4663
// plus any correlated event 4660 deletion confirmation.
type FileDeletionEvent struct {
	RecordID            uint64    `json:"record_id"`
	TimeCreated         time.Time `json:"time_created"`
	ObjectName          string    `json:"object_name"`
	ProcessName         string    `json:"process_name"`
	ProcessID           string    `json:"process_id"`
	HandleID            string    `json:"handle_id"`
	AccessMask          string    `json:"access_mask"`
	AccessList          string    `json:"access_list"`
	DeletionConfirmed   bool      `json:"deletion_confirmed"`
	DeletionRecordID    uint64    `json:"deletion_record_id,omitempty"`
	DeletionTimeCreated time.Time `json:"deletion_time_created,omitempty"`
}

// DeletionEventClassification explains whether an observed event blocks the
// test or remains diagnostic.
type DeletionEventClassification string

const (
	// DeletionBlocksControlledInstaller identifies an attributed system-file deletion.
	DeletionBlocksControlledInstaller DeletionEventClassification = "blocking-controlled-installer-system-delete"
	// DeletionOutsideOperation identifies an event outside the operation's record range.
	DeletionOutsideOperation DeletionEventClassification = "diagnostic-outside-operation-window"
	// DeletionNonDeleteAccess identifies a valid access mask without deletion rights.
	DeletionNonDeleteAccess DeletionEventClassification = "diagnostic-non-delete-access"
	// DeletionOutsideSystemRoot identifies a path outside the monitored root.
	DeletionOutsideSystemRoot DeletionEventClassification = "diagnostic-outside-system-root"
	// DeletionExcludedSystemPath identifies a deliberately volatile path.
	DeletionExcludedSystemPath DeletionEventClassification = "diagnostic-excluded-system-path"
	// DeletionUncontrolledProcess identifies an event from an unrelated process.
	DeletionUncontrolledProcess DeletionEventClassification = "diagnostic-uncontrolled-process"
	// DeletionUnconfirmedAccess identifies DELETE access without event 4660 confirmation.
	DeletionUnconfirmedAccess DeletionEventClassification = "diagnostic-unconfirmed-delete-access"
	// DeletionInvalidEvidence identifies an event that cannot be classified safely.
	DeletionInvalidEvidence DeletionEventClassification = "error-invalid-evidence"
)

// ClassifiedDeletionEvent retains the original evidence and its disposition.
type ClassifiedDeletionEvent struct {
	Event          FileDeletionEvent           `json:"event"`
	Classification DeletionEventClassification `json:"classification"`
	Blocks         bool                        `json:"blocks"`
	EvidenceError  string                      `json:"evidence_error,omitempty"`
}

type eventXML struct {
	System struct {
		EventID       uint64 `xml:"EventID"`
		EventRecordID uint64 `xml:"EventRecordID"`
		TimeCreated   struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	EventData []struct {
		Name  string `xml:"Name,attr"`
		Value string `xml:",chardata"`
	} `xml:"EventData>Data"`
}

type eventsXML struct {
	XMLName xml.Name   `xml:"Events"`
	Events  []eventXML `xml:"Event"`
}

// DecodeFileDeletionEventsXML decodes the rooted XML emitted by wevtutil with
// /format:xml /element:Events without consulting the localized Message field.
// Event 4663 supplies the object name and delete access; event 4660 confirms
// that the same process and handle actually deleted the object.
func DecodeFileDeletionEventsXML(output string) ([]FileDeletionEvent, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var envelope eventsXML
	if err := xml.Unmarshal([]byte(output), &envelope); err != nil {
		return nil, fmt.Errorf("decode Security event XML: %w", err)
	}

	type deletionConfirmation struct {
		recordID    uint64
		timeCreated time.Time
	}
	confirmations := make(map[string][]deletionConfirmation)
	events := make([]FileDeletionEvent, 0, len(envelope.Events))
	for _, raw := range envelope.Events {
		if raw.System.EventID != 4660 && raw.System.EventID != 4663 {
			continue
		}
		if raw.System.EventRecordID == 0 {
			return nil, fmt.Errorf("event %d is missing EventRecordID", raw.System.EventID)
		}
		created, err := time.Parse(time.RFC3339Nano, raw.System.TimeCreated.SystemTime)
		if err != nil {
			return nil, fmt.Errorf("decode event %d record %d timestamp %q: %w", raw.System.EventID, raw.System.EventRecordID, raw.System.TimeCreated.SystemTime, err)
		}
		data := eventData(raw)
		if raw.System.EventID == 4660 {
			processID := data["ProcessId"]
			handleID := data["HandleId"]
			if processID == "" || handleID == "" {
				return nil, fmt.Errorf("event 4660 record %d is missing ProcessId or HandleId", raw.System.EventRecordID)
			}
			if isUsableHandleID(handleID) {
				key := processHandleKey(processID, handleID)
				confirmations[key] = append(confirmations[key], deletionConfirmation{recordID: raw.System.EventRecordID, timeCreated: created})
			}
			continue
		}
		events = append(events, FileDeletionEvent{
			RecordID:    raw.System.EventRecordID,
			TimeCreated: created,
			ObjectName:  data["ObjectName"],
			ProcessName: data["ProcessName"],
			ProcessID:   data["ProcessId"],
			HandleID:    data["HandleId"],
			AccessMask:  data["AccessMask"],
			AccessList:  data["AccessList"],
		})
	}

	for index := range events {
		event := &events[index]
		for _, confirmation := range confirmations[processHandleKey(event.ProcessID, event.HandleID)] {
			if confirmation.recordID <= event.RecordID {
				continue
			}
			event.DeletionConfirmed = true
			event.DeletionRecordID = confirmation.recordID
			event.DeletionTimeCreated = confirmation.timeCreated
			break
		}
	}
	return events, nil
}

func eventData(raw eventXML) map[string]string {
	data := make(map[string]string, len(raw.EventData))
	for _, item := range raw.EventData {
		data[item.Name] = strings.TrimSpace(item.Value)
	}
	return data
}

func processHandleKey(processID, handleID string) string {
	return strings.ToLower(strings.TrimSpace(processID)) + "\x00" + strings.ToLower(strings.TrimSpace(handleID))
}

func isUsableHandleID(handleID string) bool {
	handleID = strings.ToLower(strings.TrimSpace(handleID))
	return handleID != "" && handleID != "0" && handleID != "0x0"
}

// DecodeSecurityLogCheckpoint decodes structured Security log metadata.
func DecodeSecurityLogCheckpoint(output string) (SecurityLogCheckpoint, error) {
	var checkpoint SecurityLogCheckpoint
	output = strings.TrimSpace(output)
	if output == "" {
		return checkpoint, errors.New("Security log checkpoint output is empty")
	}
	if err := json.Unmarshal([]byte(output), &checkpoint); err != nil {
		return checkpoint, fmt.Errorf("decode Security log checkpoint: %w", err)
	}
	if checkpoint.RecordID == 0 || checkpoint.OldestRecordID == 0 {
		return checkpoint, fmt.Errorf("Security log checkpoint requires non-zero newest and oldest records, got %d and %d", checkpoint.RecordID, checkpoint.OldestRecordID)
	}
	return checkpoint, nil
}

// ValidateSecurityLogWindow rejects backwards or rolled-over record ranges.
func ValidateSecurityLogWindow(start SecurityLogCheckpoint, end SecurityLogCheckpoint) error {
	if end.RecordID < start.RecordID {
		return fmt.Errorf("Security log record identifier moved backwards from %d to %d", start.RecordID, end.RecordID)
	}
	if start.RecordID > 0 && end.OldestRecordID > start.RecordID && end.OldestRecordID-start.RecordID > 1 {
		return fmt.Errorf("Security log checkpoint %d rolled out; oldest available record is %d", start.RecordID, end.OldestRecordID)
	}
	return nil
}

// HasFileDeletionAccess reports whether the access mask includes DELETE or
// FILE_DELETE_CHILD. Missing or malformed masks are evidence errors.
func HasFileDeletionAccess(event FileDeletionEvent) (bool, error) {
	accessMask := strings.TrimSpace(event.AccessMask)
	if accessMask == "" {
		return false, errors.New("event 4663 is missing AccessMask")
	}
	mask, err := strconv.ParseUint(accessMask, 0, 32)
	if err != nil {
		return false, fmt.Errorf("event 4663 has malformed AccessMask %q: %w", event.AccessMask, err)
	}
	return mask&(deleteAccess|fileDeleteChildAccess) != 0, nil
}

// ClassifyDeletionEvent classifies an event relative to one controlled MSI
// operation.
func ClassifyDeletionEvent(event FileDeletionEvent, start, end SecurityLogCheckpoint, monitoredRoot string, exclusions []string) (ClassifiedDeletionEvent, error) {
	result := ClassifiedDeletionEvent{Event: event}
	if event.RecordID <= start.RecordID || event.RecordID > end.RecordID {
		result.Classification = DeletionOutsideOperation
		return result, nil
	}

	hasDeletionAccess, err := HasFileDeletionAccess(event)
	if err != nil {
		result.Classification = DeletionInvalidEvidence
		result.EvidenceError = err.Error()
		return result, err
	}
	if !hasDeletionAccess {
		result.Classification = DeletionNonDeleteAccess
		return result, nil
	}

	if !isUsableHandleID(event.HandleID) {
		err := errors.New("event 4663 with deletion access has no usable HandleId")
		result.Classification = DeletionInvalidEvidence
		result.EvidenceError = err.Error()
		return result, err
	}
	if !event.DeletionConfirmed {
		result.Classification = DeletionUnconfirmedAccess
		return result, nil
	}

	if strings.TrimSpace(event.ObjectName) == "" {
		err := fmt.Errorf("event 4663 record %d is missing ObjectName", event.RecordID)
		result.Classification = DeletionInvalidEvidence
		result.EvidenceError = err.Error()
		return result, err
	}
	if !windowsPathWithin(event.ObjectName, monitoredRoot) {
		result.Classification = DeletionOutsideSystemRoot
		return result, nil
	}
	if windowsPathWithinAny(event.ObjectName, exclusions) {
		result.Classification = DeletionExcludedSystemPath
		return result, nil
	}

	if strings.TrimSpace(event.ProcessName) == "" {
		err := fmt.Errorf("event 4663 record %d is missing ProcessName", event.RecordID)
		result.Classification = DeletionInvalidEvidence
		result.EvidenceError = err.Error()
		return result, err
	}
	if !isControlledInstallerProcess(event.ProcessName) {
		result.Classification = DeletionUncontrolledProcess
		return result, nil
	}

	result.Classification = DeletionBlocksControlledInstaller
	result.Blocks = true
	return result, nil
}

func isControlledInstallerProcess(processName string) bool {
	normalized := normalizeWindowsPath(processName)
	name := normalized
	if index := strings.LastIndex(normalized, `\`); index >= 0 {
		name = normalized[index+1:]
	}
	return name == "msiexec.exe" || name == "dllhost.exe"
}

func windowsPathWithinAny(candidate string, roots []string) bool {
	for _, root := range roots {
		if windowsPathWithin(candidate, root) {
			return true
		}
	}
	return false
}

func windowsPathWithin(candidate, root string) bool {
	candidate = normalizeWindowsPath(candidate)
	root = strings.TrimSuffix(normalizeWindowsPath(root), `\`)
	return candidate == root || strings.HasPrefix(candidate, root+`\`)
}

func normalizeWindowsPath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", `\`)
	value = strings.TrimPrefix(value, `\\?\`)
	value = pathpkg.Clean(strings.ReplaceAll(value, `\`, "/"))
	if value == "." {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(value, "/", `\`))
}
