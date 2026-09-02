// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentcrashdetectimpl

import (
	"encoding/binary"
	"testing"

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
