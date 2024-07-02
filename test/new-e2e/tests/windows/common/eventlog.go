// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// EventLogEntry contains basic information from a Windows event log entry
type EventLogEntry struct {
	ID           int
	Level        int
	ProviderName string
	LogName      string
	Message      string
	RecordID     int
	TimeCreated  string
}

// ExportEventLog exports an event log to a file
func ExportEventLog(host *components.RemoteHost, logName string, outputPath string) error {
	remoteOutputPath, err := GetTemporaryFile(host)
	if err != nil {
		return fmt.Errorf("error getting temp file path: %w", err)
	}
	//nolint:errcheck
	defer host.Remove(remoteOutputPath)

	// must pass /overwrite b/c the temporary file is already created
	cmd := fmt.Sprintf("wevtutil.exe export-log '%s' '%s' /overwrite", logName, remoteOutputPath)
	_, err = host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("error exporting %s event log: %w", logName, err)
	}

	err = host.GetFile(remoteOutputPath, outputPath)
	if err != nil {
		return fmt.Errorf("error getting exported %s event log file %s: %w", logName, remoteOutputPath, err)
	}

	return nil
}

// ClearEventLog clears an event log
func ClearEventLog(host *components.RemoteHost, logName string) error {
	cmd := fmt.Sprintf("wevtutil.exe clear-log '%s'", logName)
	_, err := host.Execute(cmd)
	if err != nil {
		return fmt.Errorf("error clearing %s event log: %w", logName, err)
	}

	return nil
}

// GetEventLogErrorsAndWarnings returns a formatted list of errors and warnings from an event log useful for display/logging
func GetEventLogErrorsAndWarnings(host *components.RemoteHost, logName string) (string, error) {
	// Format events with Format-List and use Out-String to avoid truncation/word-wrap
	cmd := fmt.Sprintf(`Get-WinEvent -FilterHashTable @{ LogName='%s'; Level=1,2,3 } -ErrorAction Stop | Select TimeCreated,RecordID,ProviderName,ID,Level,Message | Format-List | Out-String -Width 4096`, logName)
	out, err := runCommandAndIgnoreNoEventsError(host, cmd)
	if err != nil {
		return "", fmt.Errorf("error getting errors and warnings from %s event log: %w", logName, err)
	}

	return out, nil
}

// runs provided command and ignores "No events were found that match the specified selection criteria" error.
// command must include `-ErrorAction Stop` to ensure that the error is raised.
func runCommandAndIgnoreNoEventsError(host *components.RemoteHost, cmd string) (string, error) {
	// ignore powershell exception if no events are found
	cmd = fmt.Sprintf(`
	try {
		%s
	} catch [Exception] {
		if ($_.Exception -match "No events were found that match the specified selection criteria") { Exit 0 }
		else { throw }
	}`, cmd)
	return host.Execute(cmd)
}

// GetEventLogEntries returns a list of event log entries
func GetEventLogEntries(host *components.RemoteHost, logName string) ([]EventLogEntry, error) {
	return GetEventLogEntriesWithFilterHashTable(host, fmt.Sprintf(`@{ LogName='%s' }`, logName))
}

// GetEventLogEntriesWithFilterHashTable returns a list of event log entries with a filter hash table
//
// https://learn.microsoft.com/en-us/powershell/scripting/samples/creating-get-winevent-queries-with-filterhashtable
func GetEventLogEntriesWithFilterHashTable(host *components.RemoteHost, filterHashTable string) ([]EventLogEntry, error) {
	// Call ConvertTo-JSON in this way to ensure that the output is a JSON array
	cmd := fmt.Sprintf(`ConvertTo-JSON -InputObject @(Get-WinEvent -ErrorAction Stop -FilterHashTable %s | Select ID,Level,ProviderName,LogName,Message,RecordID,TimeCreated)`, filterHashTable)
	out, err := runCommandAndIgnoreNoEventsError(host, cmd)
	if err != nil {
		return nil, fmt.Errorf("error getting '%s' event log entries: %w", filterHashTable, err)
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}

	entries := []EventLogEntry{}
	err = json.Unmarshal([]byte(out), &entries)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling '%s' event log entries: %w\n%s", filterHashTable, err, out)
	}

	return entries, nil
}

// GetEventLogErrorAndWarningEntries returns a list of error and warning event log entries
func GetEventLogErrorAndWarningEntries(host *components.RemoteHost, logName string) ([]EventLogEntry, error) {
	return GetEventLogEntriesWithFilterHashTable(host, fmt.Sprintf(`@{ LogName='%s'; Level=1,2,3 }`, logName))
}

// GetEventLogEntriesFromProvider returns a list of event log entries from a specific provider
func GetEventLogEntriesFromProvider(host *components.RemoteHost, logName string, providerName string) ([]EventLogEntry, error) {
	return GetEventLogEntriesWithFilterHashTable(host, fmt.Sprintf(`@{ LogName='%s'; ProviderName='%s' }`, logName, providerName))
}
