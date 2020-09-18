// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package decoder

var (
	// Utf16leEOL is the bytes sequence for UTF-16 Little-Endian end-of-line char
	Utf16leEOL = []byte{'\n', 0x00}
	// Utf16beEOL is the bytes sequence for UTF-16 Big-Endian end-of-line char
	Utf16beEOL = []byte{0x00, '\n'}
)

// EndLineMatcher defines the criterion to whether to end a line or not.
type EndLineMatcher interface {
	// Match takes the existing bytes and the bytes to be appended, returns
	// true if the combination matches the end of line condition.
	Match(exists []byte, appender []byte, start int, end int) bool
	SeparatorLen() int
}

// NewLineMatcher implements EndLineMatcher for line ending with '\n'
type NewLineMatcher struct {
	EndLineMatcher
}

// Match returns true whenever a '\n' (newline) is met.
func (n *NewLineMatcher) Match(exists []byte, appender []byte, start int, end int) bool {
	return appender[end] == '\n'
}

// SeparatorLen returns the length of the line separator
func (n *NewLineMatcher) SeparatorLen() int {
	return 1
}

// BytesSequenceMatcher defines the criterion to whether to end a line based on an arbitrary byte sequence
type BytesSequenceMatcher struct {
	sequence []byte
}

// NewBytesSequenceMatcher Returns a new matcher based on custom bytes sequence
func NewBytesSequenceMatcher(sequence []byte) *BytesSequenceMatcher {
	return &BytesSequenceMatcher{sequence}
}

// Match returns true whenever it finds a matching sequence at appender[end+1-len(b.sequence):end+1]
func (b *BytesSequenceMatcher) Match(exists []byte, appender []byte, start int, end int) bool {
	// Total read message is append(exists,appender[start:end]) and the decoder just read appender[end]
	// Thus the separator sequence is checked against append(exists, appender[start:end+1])
	l := len(exists) + ((end + 1) - start)
	if l < len(b.sequence) {
		return false
	}
	for i := 1; i <= len(b.sequence); i++ {
		seqIdx := len(b.sequence) - i
		idxToCheck := l - i
		if !b.checkByte(exists, appender[start:end+1], idxToCheck, b.sequence[seqIdx]) {
			return false
		}
	}
	return true
}

// SeparatorLen return the number of byte to ignore
func (b *BytesSequenceMatcher) SeparatorLen() int {
	return len(b.sequence)
}

func (b *BytesSequenceMatcher) checkByte(exists []byte, bs []byte, i int, val byte) bool {
	l := len(exists) + len(bs)
	if i < l {
		if i < len(exists) {
			return exists[i] == val
		}
		return bs[i-len(exists)] == val
	}
	return false
}
