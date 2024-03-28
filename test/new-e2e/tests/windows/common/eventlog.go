// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

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

// GetEventLogErrorsAndWarnings returns a formatted list of errors and warnings from an event log
func GetEventLogErrorsAndWarnings(host *components.RemoteHost, logName string) (string, error) {
	// ignore powershell exception if no events are found
	cmd := fmt.Sprintf(`
	try {
		Get-WinEvent -FilterHashTable @{ LogName='%s'; Level=1,2,3 } -ErrorAction Stop | Select TimeCreated,RecordID,ProviderName,ID,Level,Message | fl
	} catch [Exception] {
		if ($_.Exception -match "No events were found that match the specified selection criteria") { Exit 0 }
		else { throw }
	}`, logName)
	out, err := host.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("error getting errors and warnings from %s event log: %w", logName, err)
	}

	return out, nil
}
