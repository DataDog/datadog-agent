// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentcrashdetectimpl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	duringInjectionMessage = "Injection-related crash during injection detected"
	postInjectionMessage   = "Injection-related crash post injection detected"
	ddInjectorFixedDataLen = 4 + 4 + 8 // ProcessId, ExitStatus, ElapsedMs
)

type ddInjectorCrashEvent struct {
	ProcessName      string `json:"process_name,omitempty"`
	ProcessID        uint32 `json:"process_id"`
	ExitStatus       string `json:"exit_status"`
	ElapsedMs        int64  `json:"elapsed_ms"`
	Phase            string `json:"phase"`
	EventsSuppressed uint64 `json:"events_suppressed,omitempty"`
}

func ddInjectorCrashPhase(message string) (string, bool) {
	switch message {
	case duringInjectionMessage:
		return "during_injection", true
	case postInjectionMessage:
		return "post_injection", true
	default:
		return "", false
	}
}

func processBaseName(processName string) string {
	processName = strings.TrimRight(processName, "\\/")
	if separator := strings.LastIndexAny(processName, "\\/"); separator >= 0 {
		return processName[separator+1:]
	}
	return processName
}

func decodeDDInjectorCrashUserData(data []byte) (ddInjectorCrashEvent, error) {
	messageEnd := bytes.IndexByte(data, 0)
	if messageEnd < 0 {
		return ddInjectorCrashEvent{}, errors.New("Message is not null terminated")
	}
	message := string(data[:messageEnd])
	phase, ok := ddInjectorCrashPhase(message)
	if !ok {
		return ddInjectorCrashEvent{}, fmt.Errorf("unexpected Message value %q", message)
	}

	offset := messageEnd + 1
	remaining := len(data) - offset
	if remaining < ddInjectorFixedDataLen {
		return ddInjectorCrashEvent{}, fmt.Errorf("event data has %d bytes after Message, expected at least %d", remaining, ddInjectorFixedDataLen)
	}

	processName := ""
	if remaining > ddInjectorFixedDataLen {
		processNameDataLen := remaining - ddInjectorFixedDataLen
		processNameBytes, err := countedWideStringContents(data[offset : offset+processNameDataLen])
		if err != nil {
			return ddInjectorCrashEvent{}, fmt.Errorf("ProcessName: %w", err)
		}
		if len(processNameBytes)+2 != processNameDataLen {
			return ddInjectorCrashEvent{}, errors.New("ProcessName length does not match the event layout")
		}
		processName = processBaseName(decodeUTF16LE(processNameBytes))
		offset += processNameDataLen
	}

	processID := binary.LittleEndian.Uint32(data[offset:])
	exitStatus := binary.LittleEndian.Uint32(data[offset+4:])
	elapsedMs := int64(binary.LittleEndian.Uint64(data[offset+8:]))
	return ddInjectorCrashEvent{
		ProcessName: processName,
		ProcessID:   processID,
		ExitStatus:  fmt.Sprintf("0x%08x", exitStatus),
		ElapsedMs:   elapsedMs,
		Phase:       phase,
	}, nil
}

func decodeUTF16LE(value []byte) string {
	codeUnits := make([]uint16, len(value)/2)
	for i := range codeUnits {
		codeUnits[i] = binary.LittleEndian.Uint16(value[i*2:])
	}
	return string(utf16.Decode(codeUnits))
}

// TraceLoggingCountedWideString encodes a little-endian uint16 byte count
// immediately before the UTF-16 contents.
func countedWideStringContents(value []byte) ([]byte, error) {
	if len(value) < 2 {
		return nil, errors.New("counted wide string is missing its length")
	}
	length := int(binary.LittleEndian.Uint16(value))
	if length%2 != 0 {
		return nil, errors.New("counted wide string has an odd UTF-16 byte length")
	}
	if length > len(value)-2 {
		return nil, errors.New("counted wide string length exceeds the property data")
	}
	return value[2 : length+2], nil
}
