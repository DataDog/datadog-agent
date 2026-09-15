// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentcrashdetectimpl

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/assert"
)

func TestDDInjectorCrashPhase(t *testing.T) {
	tests := []struct {
		message string
		phase   string
		ok      bool
	}{
		{duringInjectionMessage, "during_injection", true},
		{postInjectionMessage, "post_injection", true},
		{"unknown", "", false},
	}

	for _, test := range tests {
		phase, ok := ddInjectorCrashPhase(test.message)
		assert.Equal(t, test.phase, phase)
		assert.Equal(t, test.ok, ok)
	}
}

func TestCountedWideStringContents(t *testing.T) {
	contents := []byte{'a', 0, 'b', 0}

	byteCounted := make([]byte, len(contents)+2)
	binary.LittleEndian.PutUint16(byteCounted, uint16(len(contents)))
	copy(byteCounted[2:], contents)
	decoded, err := countedWideStringContents(byteCounted)
	assert.NoError(t, err)
	assert.Equal(t, contents, decoded)

	_, err = countedWideStringContents([]byte{4, 0, 'a', 0})
	assert.Error(t, err)
	_, err = countedWideStringContents([]byte{1, 0, 'a'})
	assert.Error(t, err)
}

func TestProcessBaseName(t *testing.T) {
	assert.Equal(t, "service.exe", processBaseName(`\Device\HarddiskVolume3\apps\service.exe`))
	assert.Equal(t, "service.exe", processBaseName(`C:\apps\service.exe`))
	assert.Equal(t, "service.exe", processBaseName("/apps/service.exe"))
	assert.Equal(t, "service.exe", processBaseName("service.exe"))
	assert.Empty(t, processBaseName(""))
}

func TestDecodeDDInjectorCrashUserData(t *testing.T) {
	t.Run("with process name", func(t *testing.T) {
		data := crashUserData(postInjectionMessage, `\Device\HarddiskVolume3\apps\crashy.exe`, 4242, 0xc0000005, 123)
		event, err := decodeDDInjectorCrashUserData(data)
		assert.NoError(t, err)
		assert.Equal(t, ddInjectorCrashEvent{
			ProcessName: "crashy.exe",
			ProcessID:   4242,
			ExitStatus:  "0xc0000005",
			ElapsedMs:   123,
			Phase:       "post_injection",
		}, event)
	})

	t.Run("without process name", func(t *testing.T) {
		data := crashUserData(duringInjectionMessage, "", 7, 0xc0000409, 42)
		event, err := decodeDDInjectorCrashUserData(data)
		assert.NoError(t, err)
		assert.Equal(t, ddInjectorCrashEvent{
			ProcessID:  7,
			ExitStatus: "0xc0000409",
			ElapsedMs:  42,
			Phase:      "during_injection",
		}, event)
	})
}

func TestDecodeDDInjectorCrashUserDataRejectsMalformedData(t *testing.T) {
	valid := crashUserData(postInjectionMessage, "crashy.exe", 1, 2, 3)
	tests := map[string][]byte{
		"missing message terminator": []byte(postInjectionMessage),
		"unknown message":            crashUserData("unknown", "", 1, 2, 3),
		"truncated fixed data":       append([]byte(postInjectionMessage), 0, 1, 2, 3),
		"truncated process name":     valid[:len(valid)-17],
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeDDInjectorCrashUserData(data)
			assert.Error(t, err)
		})
	}
}

func crashUserData(message, processName string, processID, exitStatus uint32, elapsedMs int64) []byte {
	data := append([]byte(message), 0)
	if processName != "" {
		codeUnits := utf16.Encode([]rune(processName))
		processNameData := make([]byte, 2+len(codeUnits)*2)
		binary.LittleEndian.PutUint16(processNameData, uint16(len(codeUnits)*2))
		for i, codeUnit := range codeUnits {
			binary.LittleEndian.PutUint16(processNameData[2+i*2:], codeUnit)
		}
		data = append(data, processNameData...)
	}
	fixedData := make([]byte, ddInjectorFixedDataLen)
	binary.LittleEndian.PutUint32(fixedData, processID)
	binary.LittleEndian.PutUint32(fixedData[4:], exitStatus)
	binary.LittleEndian.PutUint64(fixedData[8:], uint64(elapsedMs))
	return append(data, fixedData...)
}
