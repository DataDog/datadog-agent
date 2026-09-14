// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package filedeletionaudit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFileDeletionEventsXMLProductionShape(t *testing.T) {
	// This is the rooted shape emitted by wevtutil with
	// /format:xml /element:Events. Multiple events verify that the decoder does
	// not silently bind only the first event as the document root.
	const input = `<?xml version="1.0" encoding="utf-8"?>
<Events>
  <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
    <System>
      <EventID>4663</EventID>
      <TimeCreated SystemTime="2026-02-19T14:03:04.1234567Z" />
      <EventRecordID>98765</EventRecordID>
    </System>
    <EventData>
      <Data Name="ObjectName">C:\Windows\System32\example.dll</Data>
      <Data Name="ProcessId">0x4bc</Data>
      <Data Name="ProcessName">C:\Windows\System32\msiexec.exe</Data>
      <Data Name="HandleId">0x111</Data>
      <Data Name="AccessList">%%1537</Data>
      <Data Name="AccessMask">0x10000</Data>
    </EventData>
  </Event>
  <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
    <System>
      <EventID>4663</EventID>
      <TimeCreated SystemTime="2026-02-19T14:03:05Z" />
      <EventRecordID>98766</EventRecordID>
    </System>
    <EventData>
      <Data Name="ObjectName">C:\Windows\System32\second.dll</Data>
      <Data Name="ProcessId">0x4bd</Data>
      <Data Name="ProcessName">C:\Windows\System32\dllhost.exe</Data>
      <Data Name="HandleId">0x222</Data>
      <Data Name="AccessList">%%1537</Data>
      <Data Name="AccessMask">0x40</Data>
    </EventData>
  </Event>
  <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
    <System><EventID>4660</EventID><TimeCreated SystemTime="2026-02-19T14:03:06Z"/><EventRecordID>98767</EventRecordID></System>
    <EventData><Data Name="ProcessId">0x4bc</Data><Data Name="HandleId">0x111</Data></EventData>
  </Event>
  <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
    <System><EventID>4660</EventID><TimeCreated SystemTime="2026-02-19T14:03:07Z"/><EventRecordID>98768</EventRecordID></System>
    <EventData><Data Name="ProcessId">0x4bd</Data><Data Name="HandleId">0x222</Data></EventData>
  </Event>
</Events>`

	events, err := DecodeFileDeletionEventsXML(input)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, FileDeletionEvent{
		RecordID:            98765,
		TimeCreated:         time.Date(2026, 2, 19, 14, 3, 4, 123456700, time.UTC),
		ObjectName:          `C:\Windows\System32\example.dll`,
		ProcessName:         `C:\Windows\System32\msiexec.exe`,
		ProcessID:           "0x4bc",
		HandleID:            "0x111",
		AccessMask:          "0x10000",
		AccessList:          "%%1537",
		DeletionConfirmed:   true,
		DeletionRecordID:    98767,
		DeletionTimeCreated: time.Date(2026, 2, 19, 14, 3, 6, 0, time.UTC),
	}, events[0])
	assert.Equal(t, uint64(98766), events[1].RecordID)
}

func TestDecodeFileDeletionEventsXMLDoesNotCorrelateDifferentProcessOrHandle(t *testing.T) {
	const input = `<Events>
	<Event><System><EventID>4663</EventID><TimeCreated SystemTime="2026-01-01T00:00:00Z"/><EventRecordID>10</EventRecordID></System><EventData><Data Name="ProcessId">0x1</Data><Data Name="HandleId">0x2</Data><Data Name="AccessMask">0x10000</Data></EventData></Event>
	<Event><System><EventID>4660</EventID><TimeCreated SystemTime="2026-01-01T00:00:01Z"/><EventRecordID>11</EventRecordID></System><EventData><Data Name="ProcessId">0x9</Data><Data Name="HandleId">0x2</Data></EventData></Event>
	<Event><System><EventID>4660</EventID><TimeCreated SystemTime="2026-01-01T00:00:02Z"/><EventRecordID>12</EventRecordID></System><EventData><Data Name="ProcessId">0x1</Data><Data Name="HandleId">0x9</Data></EventData></Event>
</Events>`
	events, err := DecodeFileDeletionEventsXML(input)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.False(t, events[0].DeletionConfirmed)
}

func TestDecodeFileDeletionEventsXMLRejectsRootlessProductionOutput(t *testing.T) {
	const input = `<Event><System><EventID>4663</EventID></System></Event><Event><System><EventID>4663</EventID></System></Event>`
	_, err := DecodeFileDeletionEventsXML(input)
	assert.ErrorContains(t, err, "expected element type <Events>")
}

func TestDecodeFileDeletionEventsXMLIgnoresOtherEvents(t *testing.T) {
	const input = `<Events><Event><System><EventID>4624</EventID><EventRecordID>3</EventRecordID><TimeCreated SystemTime="2026-01-01T00:00:00Z"/></System></Event></Events>`
	events, err := DecodeFileDeletionEventsXML(input)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestDecodeFileDeletionEventsXMLEmptyAndMalformed(t *testing.T) {
	events, err := DecodeFileDeletionEventsXML("  \n")
	require.NoError(t, err)
	assert.Nil(t, events)

	_, err = DecodeFileDeletionEventsXML(`<Events><Event>`)
	assert.ErrorContains(t, err, "decode Security event XML")
}

func TestDecodeFileDeletionEventsXMLRejectsMissingMetadata(t *testing.T) {
	_, err := DecodeFileDeletionEventsXML(`<Events><Event><System><EventID>4663</EventID><TimeCreated SystemTime="2026-01-01T00:00:00Z"/></System></Event></Events>`)
	assert.ErrorContains(t, err, "missing EventRecordID")

	_, err = DecodeFileDeletionEventsXML(`<Events><Event><System><EventID>4663</EventID><EventRecordID>4</EventRecordID><TimeCreated SystemTime="not-a-time"/></System></Event></Events>`)
	assert.ErrorContains(t, err, "timestamp")
}

func TestDecodeSecurityLogCheckpoint(t *testing.T) {
	checkpoint, err := DecodeSecurityLogCheckpoint(`{"RecordID":120,"OldestRecordID":40,"RecordCount":81,"FileSize":2048,"MaximumSizeInBytes":4096}`)
	require.NoError(t, err)
	assert.Equal(t, SecurityLogCheckpoint{
		RecordID:           120,
		OldestRecordID:     40,
		RecordCount:        81,
		FileSize:           2048,
		MaximumSizeInBytes: 4096,
	}, checkpoint)
}

func TestDecodeSecurityLogCheckpointErrors(t *testing.T) {
	_, err := DecodeSecurityLogCheckpoint("")
	assert.ErrorContains(t, err, "empty")

	_, err = DecodeSecurityLogCheckpoint("not-json")
	assert.ErrorContains(t, err, "decode Security log checkpoint")

	_, err = DecodeSecurityLogCheckpoint(`{"RecordID":120,"OldestRecordID":0}`)
	assert.ErrorContains(t, err, "non-zero newest and oldest")

	_, err = DecodeSecurityLogCheckpoint(`{"RecordID":0,"OldestRecordID":0}`)
	assert.ErrorContains(t, err, "non-zero newest and oldest")
}

func TestValidateSecurityLogWindow(t *testing.T) {
	start := SecurityLogCheckpoint{RecordID: 100, OldestRecordID: 10}
	assert.NoError(t, ValidateSecurityLogWindow(start, SecurityLogCheckpoint{RecordID: 110, OldestRecordID: 100}))
	assert.NoError(t, ValidateSecurityLogWindow(start, SecurityLogCheckpoint{RecordID: 110, OldestRecordID: 101}))

	err := ValidateSecurityLogWindow(start, SecurityLogCheckpoint{RecordID: 110, OldestRecordID: 102})
	assert.ErrorContains(t, err, "rolled out")

	err = ValidateSecurityLogWindow(start, SecurityLogCheckpoint{RecordID: 99, OldestRecordID: 10})
	assert.ErrorContains(t, err, "moved backwards")
}

func TestHasFileDeletionAccess(t *testing.T) {
	for _, mask := range []string{"0x10000", "0x40", "0x10040"} {
		hasAccess, err := HasFileDeletionAccess(FileDeletionEvent{AccessMask: mask})
		require.NoError(t, err)
		assert.True(t, hasAccess)
	}

	hasAccess, err := HasFileDeletionAccess(FileDeletionEvent{AccessMask: "0x1"})
	require.NoError(t, err)
	assert.False(t, hasAccess)

	_, err = HasFileDeletionAccess(FileDeletionEvent{})
	assert.ErrorContains(t, err, "missing AccessMask")
	_, err = HasFileDeletionAccess(FileDeletionEvent{AccessMask: "malformed"})
	assert.ErrorContains(t, err, "malformed AccessMask")
}

func TestClassifyDeletionEvent(t *testing.T) {
	start := SecurityLogCheckpoint{RecordID: 100}
	end := SecurityLogCheckpoint{RecordID: 200}
	base := FileDeletionEvent{
		RecordID:          150,
		ObjectName:        `C:\Windows\System32\owned.dll`,
		ProcessName:       `C:\Windows\System32\msiexec.exe`,
		ProcessID:         "0xabc",
		HandleID:          "0x123",
		AccessMask:        "0x10000",
		DeletionConfirmed: true,
	}

	tests := []struct {
		name           string
		modify         func(*FileDeletionEvent)
		exclusions     []string
		classification DeletionEventClassification
		blocks         bool
	}{
		{name: "installer delete blocks", classification: DeletionBlocksControlledInstaller, blocks: true},
		{
			name: "dllhost delete-child blocks with case and separator variations",
			modify: func(event *FileDeletionEvent) {
				event.ProcessName = `c:/WINDOWS/system32/DLLHOST.EXE`
				event.ObjectName = `c:/WINDOWS/System32/subdir/file.dll`
				event.AccessMask = "0x40"
			},
			classification: DeletionBlocksControlledInstaller,
			blocks:         true,
		},
		{
			name: "servicing process remains diagnostic",
			modify: func(event *FileDeletionEvent) {
				event.ProcessName = `C:\Windows\servicing\TrustedInstaller.exe`
			},
			classification: DeletionUncontrolledProcess,
		},
		{
			name: "record at starting boundary is outside operation",
			modify: func(event *FileDeletionEvent) {
				event.RecordID = 100
			},
			classification: DeletionOutsideOperation,
		},
		{
			name: "record after ending boundary is outside operation",
			modify: func(event *FileDeletionEvent) {
				event.RecordID = 201
			},
			classification: DeletionOutsideOperation,
		},
		{
			name: "read access remains diagnostic",
			modify: func(event *FileDeletionEvent) {
				event.AccessMask = "0x1"
			},
			classification: DeletionNonDeleteAccess,
		},
		{
			name: "unconfirmed delete access remains diagnostic",
			modify: func(event *FileDeletionEvent) {
				event.DeletionConfirmed = false
			},
			classification: DeletionUnconfirmedAccess,
		},
		{
			name: "excluded root remains diagnostic",
			modify: func(event *FileDeletionEvent) {
				event.ObjectName = `C:\WINDOWS\Installer\cache.tmp`
			},
			exclusions:     []string{`c:/windows/installer/`},
			classification: DeletionExcludedSystemPath,
		},
		{
			name: "exclusion uses component boundary",
			modify: func(event *FileDeletionEvent) {
				event.ObjectName = `C:\Windows\InstallerBackup\owned.dll`
			},
			exclusions:     []string{`C:\Windows\Installer`},
			classification: DeletionBlocksControlledInstaller,
			blocks:         true,
		},
		{
			name: "system root uses component boundary",
			modify: func(event *FileDeletionEvent) {
				event.ObjectName = `C:\Windows.old\System32\owned.dll`
			},
			classification: DeletionOutsideSystemRoot,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := base
			if test.modify != nil {
				test.modify(&event)
			}
			result, err := ClassifyDeletionEvent(event, start, end, `C:\Windows`, test.exclusions)
			require.NoError(t, err)
			assert.Equal(t, test.classification, result.Classification)
			assert.Equal(t, test.blocks, result.Blocks)
			assert.Equal(t, event.ProcessID, result.Event.ProcessID)
		})
	}
}

func TestClassifyDeletionEventRejectsMissingHandle(t *testing.T) {
	event := FileDeletionEvent{
		RecordID:    150,
		ObjectName:  `C:\Windows\System32\owned.dll`,
		ProcessName: `C:\Windows\System32\msiexec.exe`,
		AccessMask:  "0x10000",
	}
	result, err := ClassifyDeletionEvent(event, SecurityLogCheckpoint{RecordID: 100}, SecurityLogCheckpoint{RecordID: 200}, `C:\Windows`, nil)
	assert.ErrorContains(t, err, "usable HandleId")
	assert.Equal(t, DeletionInvalidEvidence, result.Classification)
}

func TestClassifyDeletionEventRejectsMalformedAccessMask(t *testing.T) {
	event := FileDeletionEvent{
		RecordID:    150,
		ObjectName:  `C:\Windows\System32\owned.dll`,
		ProcessName: `C:\Windows\System32\msiexec.exe`,
		AccessMask:  "malformed",
	}
	result, err := ClassifyDeletionEvent(event, SecurityLogCheckpoint{RecordID: 100}, SecurityLogCheckpoint{RecordID: 200}, `C:\Windows`, nil)
	assert.ErrorContains(t, err, "malformed AccessMask")
	assert.Equal(t, DeletionInvalidEvidence, result.Classification)
	assert.NotEmpty(t, result.EvidenceError)
}

func TestNormalizeWindowsPath(t *testing.T) {
	assert.Equal(t, `c:\windows\system32\file.dll`, normalizeWindowsPath(`C:/Windows//Temp/../System32/file.dll`))
	assert.Equal(t, `c:\windows\file.dll`, normalizeWindowsPath(`\\?\C:\Windows\file.dll`))
	assert.True(t, windowsPathWithin(`C:/WINDOWS/System32/file.dll`, `c:\windows`))
	assert.False(t, windowsPathWithin(`C:\WindowsOld\file.dll`, `C:\Windows`))
}
