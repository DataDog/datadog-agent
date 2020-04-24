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
	errInvalidAttribute  = errors.New("invalid attribute; length too short or too large")
	errMessageTooShort   = errors.New("netlink message is too short")
	errMissingNestedAttr = errors.New("netlink message is missing nested attribute")
)

// Based on https://github.com/mdlayher/netlink/blob/master/attribute.go
// The main optimizations here are:
// * We don't allocate a slice for the field data when we're scanning the attributes;
// * The AttributeScanner itself can be reused when scanning nested fields and decoding different messages;
type AttributeScanner struct {
	level  int
	frames []*NestedFrame
}

// A NestedFrame encapsulates the decoding information of a certain nesting level
type NestedFrame struct {
	a netlink.Attribute

	// The slice of input bytes and its iterator index.
	b []byte
	i int

	// Any error encountered while decoding attributes.
	err error
}

// NewAttributeScanner returns a new instance of AttributeScanner
func NewAttributeScanner() *AttributeScanner {
	scanner := &AttributeScanner{
		frames: make([]*NestedFrame, 3),
	}

	for i := range scanner.frames {
		scanner.frames[i] = &NestedFrame{}
	}

	return scanner
}

// Next advances the decoder to the next netlink attribute.  It returns false
// when no more attributes are present, or an error was encountered.
func (s *AttributeScanner) Next() bool {
	f := s.currentFrame()

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
	if int(f.a.Length) < nlaHeaderLen {
		f.i += nlaHeaderLen
	} else {
		f.i += nlaAlign(int(f.a.Length))
	}

	return true
}

// Type returns the Attribute.Type field of the current netlink attribute
// pointed to by the decoder.
//
// Type masks off the high bits of the netlink attribute type which may contain
// the Nested and NetByteOrder flags. These can be obtained by calling TypeFlags.
func (s *AttributeScanner) Type() uint16 {
	// Mask off any flags stored in the high bits.
	return s.currentFrame().a.Type & attrTypeMask
}

// Err returns the first error encountered by the scanner.
func (s *AttributeScanner) Err() error {
	return s.currentFrame().err
}

// Bytes returns the raw bytes of the current Attribute's data.
func (s *AttributeScanner) Bytes() []byte {
	return s.currentFrame().a.Data
}

// Nested executes the given function within a new NestedFrame
func (s *AttributeScanner) Nested(fn func() error) {
	if len(s.frames) <= s.level {
		s.frames = append(s.frames, &NestedFrame{})
	}

	prev := s.currentFrame()

	// Create new frame
	s.level++
	current := s.currentFrame()
	current.b = prev.a.Data
	current.i = 0
	current.err = nil

	// Execute function within new frame
	err := fn()
	prev.err = err

	// Pop frame
	s.level--
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

	s.currentFrame().b = data[offset:]
	return nil
}

// Nearly identical to netlink.Attribute.Unmarshal, but the Data field of the current
// attribute simply points to the underlying byte slice, instead of copying it.
func (f *NestedFrame) unmarshal() error {
	b := f.b[f.i:]

	if len(b) < nlaHeaderLen {
		return errInvalidAttribute
	}

	f.a.Length = nlenc.Uint16(b[0:2])
	f.a.Type = nlenc.Uint16(b[2:4])

	if int(f.a.Length) > len(b) {
		return errInvalidAttribute
	}

	switch {
	// No length, no data
	case f.a.Length == 0:
		f.a.Data = nil
	// Not enough length for any data
	case int(f.a.Length) < nlaHeaderLen:
		return errInvalidAttribute
	// Data present
	case int(f.a.Length) >= nlaHeaderLen:
		f.a.Data = b[nlaHeaderLen:f.a.Length]
	}

	return nil
}

func (s *AttributeScanner) currentFrame() *NestedFrame {
	return s.frames[s.level]
}

func messageOffset(data []byte) int {
	if (data[0] == unix.AF_INET || data[0] == unix.AF_INET6) && data[1] == unix.NFNETLINK_V0 {
		return 4
	}
	return 0
}
