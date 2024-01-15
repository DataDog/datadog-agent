// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ebpf holds ebpf related files
package ebpf

import (
	"bytes"
	"encoding/binary"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// BytesMapItem describes a raw table key or value
type BytesMapItem []byte

// MarshalBinary returns the binary representation of a BytesMapItem
func (i BytesMapItem) MarshalBinary() ([]byte, error) {
	return []byte(i), nil
}

// Uint8MapItem describes an uint8 table key or value
type Uint8MapItem uint8

// MarshalBinary returns the binary representation of a Uint8MapItem
func (i Uint8MapItem) MarshalBinary() ([]byte, error) {
	return []byte{uint8(i)}, nil
}

// Uint16MapItem describes an uint16 table key or value
type Uint16MapItem uint16

// MarshalBinary returns the binary representation of a Uint16MapItem
func (i Uint16MapItem) MarshalBinary() ([]byte, error) {
	b := make([]byte, 2)
	model.ByteOrder.PutUint16(b, uint16(i))
	return b, nil
}

// Uint32MapItem describes an uint32 table key or value
type Uint32MapItem uint32

// MarshalBinary returns the binary representation of a Uint32MapItem
func (i Uint32MapItem) MarshalBinary() ([]byte, error) {
	b := make([]byte, 4)
	model.ByteOrder.PutUint32(b, uint32(i))
	return b, nil
}

// Uint64MapItem describes an uint64 table key or value
type Uint64MapItem uint64

// MarshalBinary returns the binary representation of a Uint64MapItem
func (i Uint64MapItem) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	model.ByteOrder.PutUint64(b, uint64(i))
	return b, nil
}

// StringMapItem describes an string table key or value
type StringMapItem struct {
	str  string
	size int
}

// MarshalBinary returns the binary representation of a StringMapItem
func (i *StringMapItem) MarshalBinary() ([]byte, error) {
	n := i.size
	if len(i.str) < i.size {
		n = len(i.str)
	}

	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, model.ByteOrder, []byte(i.str)[0:n]); err != nil {
		return nil, err
	}
	rep := make([]byte, i.size)
	copy(rep, buffer.Bytes())
	return rep, nil
}

// NewStringMapItem returns a new StringMapItem
func NewStringMapItem(str string, size int) *StringMapItem {
	return &StringMapItem{str: str, size: size}
}

// Zero table items
var (
	ZeroUint8MapItem  = BytesMapItem([]byte{0})
	ZeroUint32MapItem = BytesMapItem([]byte{0, 0, 0, 0})
	ZeroUint64MapItem = BytesMapItem([]byte{0, 0, 0, 0, 0, 0, 0, 0})
)

var (
	// BufferSelectorSyscallMonitorKey is the key used to select the active syscall monitor buffer key
	BufferSelectorSyscallMonitorKey = ZeroUint32MapItem
	// BufferSelectorERPCMonitorKey is the key used to select the active eRPC monitor buffer key
	BufferSelectorERPCMonitorKey = Uint32MapItem(1)
	// BufferSelectorDiscarderMonitorKey is the key used to select the active discarder monitor buffer key
	BufferSelectorDiscarderMonitorKey = Uint32MapItem(2)
	// BufferSelectorApproverMonitorKey is the key used to select the active approver monitor buffer key
	BufferSelectorApproverMonitorKey = Uint32MapItem(3)
)
