// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"errors"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"golang.org/x/sys/unix"
)

// attrTypeMask masks off Type bits used for the above flags.
var attrTypeMask uint16 = 0x3fff

// errors
var (
	errInvalidAttribute = errors.New("invalid attribute; length too short or too large")
	errMessageTooShort  = errors.New("netlink message is too short")
)

// AttributeScanner provides an iterator API to traverse each field in a netlink message.
// The same AttributeScanner instance can be used multiple times with different messages by calling ResetTo().
// When scanning a netlink message, every time we "enter" in a nested field, a new NestedFrame is created.
// Based on https://github.com/mdlayher/netlink/blob/c558cf25207e57bc9cc026d2dd69e2ea2f6abd0e/attribute.go
type AttributeScanner struct {
	// level of nesting we're currently in
	level int
	// when we're scanning nested fields, each field level will have an associated frame
	frames []*NestedFrame
}

// A NestedFrame encapsulates the decoding information of a certain nesting level
type NestedFrame struct {
	// The current attribute we're looking at
	attr netlink.Attribute

	// The slice of input bytes and its iterator index.
	b []byte
	i int

	// Any error encountered while decoding attributes.
	err error
}

// NewAttributeScanner returns a new instance of AttributeScanner
func NewAttributeScanner() *AttributeScanner {
	// We pre-allocate 3 frames since the Conntrack messages have 3 levels of nesting
	scanner := &AttributeScanner{
		frames: make([]*NestedFrame, 3),
	}

	for i := range scanner.frames {
		scanner.frames[i] = &NestedFrame{}
	}

	return scanner
}

// Next advances the scanner to the next netlink attribute (within the same NestedFrame).
// It returns false when no more attributes are present, or an error was encountered.
func (s *AttributeScanner) Next() bool {
	f := s.frame()

	if f.err != nil {
		// Hit an error, stop iteration.
		return false
	}

	// Exit if array pointer is at or beyond the end of the slice.
	if f.i >= len(f.b) {
		return false
	}

	if err := f.unmarshal(); err != nil {
		f.err = err
		return false
	}

	// Advance the pointer by at least one header's length.
	if int(f.attr.Length) < nlaHeaderLen {
		f.i += nlaHeaderLen
	} else {
		f.i += nlaAlign(int(f.attr.Length))
	}

	return true
}

// Type returns the Attribute.Type field of the current netlink attribute
// pointed to by the scanner.
func (s *AttributeScanner) Type() uint16 {
	// Mask off any flags stored in the high bits.
	return s.frame().attr.Type & attrTypeMask
}

// Err returns the first error encountered by the scanner.
func (s *AttributeScanner) Err() error {
	return s.frame().err
}

// Bytes returns the raw bytes of the current Attribute's data.
func (s *AttributeScanner) Bytes() []byte {
	return s.frame().attr.Data
}

// Nested executes the given function within a new NestedFrame
func (s *AttributeScanner) Nested(fn func() error) {
	// Push new frame
	s.push()

	// Execute function within new frame
	err := fn()

	// Pop frame and assign error
	s.pop()
	s.frame().err = err
}

// ResetTo makes the current AttributeScanner ready for another netlink message
func (s *AttributeScanner) ResetTo(data []byte) error {
	s.level = 0

	for _, f := range s.frames {
		f.b = nil
		f.i = 0
		f.err = nil
	}

	if len(data) < 2 {
		return errMessageTooShort
	}

	offset := messageOffset(data[:2])

	if len(data) <= offset {
		return errMessageTooShort
	}

	s.frame().b = data[offset:]
	return nil
}

// frame returns the current frame
func (s *AttributeScanner) frame() *NestedFrame {
	return s.frames[s.level]
}

// push a new nested frame
func (s *AttributeScanner) push() {
	prev := s.frame()

	// Create a new frame object if necessary
	if len(s.frames) <= s.level {
		s.frames = append(s.frames, &NestedFrame{})
	}

	s.level++

	current := s.frame()
	current.b = prev.attr.Data
	current.i = 0
	current.err = nil
}

// pop nested frame
func (s *AttributeScanner) pop() {
	s.level--
}

// Nearly identical to netlink.Attribute.Unmarshal, but the Data field of the current
// attribute simply points to the underlying byte slice, instead of copying it.
func (f *NestedFrame) unmarshal() error {
	b := f.b[f.i:]

	if len(b) < nlaHeaderLen {
		return errInvalidAttribute
	}

	f.attr.Length = nlenc.Uint16(b[0:2])
	f.attr.Type = nlenc.Uint16(b[2:4])

	if int(f.attr.Length) > len(b) {
		return errInvalidAttribute
	}

	switch {
	// No length, no data
	case f.attr.Length == 0:
		f.attr.Data = nil
	// Not enough length for any data
	case int(f.attr.Length) < nlaHeaderLen:
		return errInvalidAttribute
	// Data present
	case int(f.attr.Length) >= nlaHeaderLen:
		f.attr.Data = b[nlaHeaderLen:f.attr.Length]
	}

	return nil
}

func messageOffset(data []byte) int {
	if (data[0] == unix.AF_INET || data[0] == unix.AF_INET6) && data[1] == unix.NFNETLINK_V0 {
		return 4
	}
	return 0
}
