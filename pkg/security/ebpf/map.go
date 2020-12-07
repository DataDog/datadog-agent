// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package ebpf

import (
	"bytes"
	"encoding/binary"
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

// Uint32MapItem describes an uint32 table key or value
type Uint32MapItem uint32

// MarshalBinary returns the binary representation of a Uint32MapItem
func (i Uint32MapItem) MarshalBinary() ([]byte, error) {
	b := make([]byte, 4)
	GetHostByteOrder().PutUint32(b, uint32(i))
	return b, nil
}

// Uint64MapItem describes an uint64 table key or value
type Uint64MapItem uint64

// MarshalBinary returns the binary representation of a Uint64MapItem
func (i Uint64MapItem) MarshalBinary() ([]byte, error) {
	b := make([]byte, 8)
	GetHostByteOrder().PutUint64(b, uint64(i))
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
	if err := binary.Write(buffer, GetHostByteOrder(), []byte(i.str)[0:n]); err != nil {
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
