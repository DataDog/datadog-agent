// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package output

// This file must be kept in sync with the ../ebpf/framing.h file.
// If adding new structure, update framing_align_test.go to check that structure
// memory layout.

// EventHeader is the message header used for the event program.
type EventHeader struct {
	DataByteLen   uint32
	ProgID        uint32
	StackBytes    uint16
	StackHashOrPC uint64
	KTimeNS       uint64
}

// DataItemHeader is the message header used for the event program.
type DataItemHeader struct {
	Type uint32
	Len  uint32
	Addr uint64
}
