// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentcrashdetectimpl

import (
	"encoding/binary"
	"errors"
	"strings"
)

const (
	duringInjectionMessage = "Injection-related crash during injection detected"
	postInjectionMessage   = "Injection-related crash post injection detected"
)

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
