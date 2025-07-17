// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package framer supports efficiently breaking chunks of binary data into frames.
package framer

import (
	"bytes"
	"fmt"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Framing describes the kind of framing applied to the byte stream being broken.
type Framing int

// Framing values.
const (
	// Newline-terminated text in UTF-8.  This also applies to ASCII and
	// single-byte extended ASCII encodings such as latin-1.
	UTF8Newline Framing = iota

	// Newline-terminated text in UTF-16-BE.
	UTF16BENewline

	// Newline-terminated text in UTF-16-LE.
	UTF16LENewline

	// Newline-terminated text in SHIFT-JIS.
	SHIFTJISNewline

	// NoFraming considers the given input as already framed.
	NoFraming

	// Docker log-stream format.
	//
	// WARNING: This bundles multiple docker frames together into a single "log
	// frame", looking for a utf-8 newline in the output.  All 8-byte binary
	// headers are included in the log frame.  The size in those headers is not
	// consulted.  The result does not include the trailing newlines.
	DockerStream
)

// Framer gets chunks of bytes (via Process(..)) and uses an
// EndLineMatcher to break those into frames, passing the results to its
// outputFn.
type Framer struct {
	// outputFn is called with each complete "line"
	outputFn func(input *message.Message, rawDataLen int)

	// the matcher is the
	matcher FrameMatcher

	// buffer is the buffer containing the bytes given to Process so far
	buffer bytes.Buffer

	// bytesFramed is the length, in bytes, of the prefix of buffer that
	// has already been output as a frame.
	bytesFramed int

	// The number of raw frames decoded from the input before they are processed.
	frames *atomic.Int64

	// contentLenLimit is the longest content value the Framer will produce.
	// Over this size, the framer will break the bytes into individual frames
	// of this size with no delimiting.
	contentLenLimit int
}

// NewFramer initializes a Framer.
//
// The framer will break the input stream into messages using the given framing.
//
// Content longer than the given limit will be broken into frames at
// that length, regardless of framing.
//
// Each frame will be passed to outputFn, including both the content of the frame
// itself and the number of raw bytes matched to represent that frame.  In general,
// the content does not contain framing data like newlines.
func NewFramer(
	outputFn func(input *message.Message, rawDataLen int),
	framing Framing,
	contentLenLimit int,
) *Framer {
	var matcher FrameMatcher
	switch framing {
	case UTF8Newline:
		matcher = &oneByteNewLineMatcher{contentLenLimit}
	case UTF16BENewline:
		matcher = &twoByteNewLineMatcher{contentLenLimit: contentLenLimit, newline: Utf16beEOL}
	case UTF16LENewline:
		matcher = &twoByteNewLineMatcher{contentLenLimit: contentLenLimit, newline: Utf16leEOL}
	case SHIFTJISNewline:
		// No special handling required for the newline matcher since Shift JIS does not use
		// newline characters (0x0a) as the second byte of a multibyte sequence.
		matcher = &oneByteNewLineMatcher{contentLenLimit}
	case DockerStream:
		matcher = &dockerStreamMatcher{contentLenLimit}
	case NoFraming:
		matcher = &noFramingMatcher{}
	default:
		panic(fmt.Sprintf("unknown framing %d", framing))
	}

	return &Framer{
		frames:          atomic.NewInt64(0),
		outputFn:        outputFn,
		matcher:         matcher,
		buffer:          bytes.Buffer{},
		bytesFramed:     0,
		contentLenLimit: contentLenLimit,
	}
}

// GetFrameCount gets the number of frames this framer has processed.  This is safe to
// call from any goroutine.
func (fr *Framer) GetFrameCount() int64 {
	return fr.frames.Load()
}

// Process handles an incoming chunk of data.  It will call outputFn for any recognized frames.  Partial
// frames are maintained between calls to Process.  The passed buffer is not used after return.
func (fr *Framer) Process(input *message.Message) {
	// we can only process unstructured message in the framer
	// TODO(remy): the same way the MultiLineHandler use the first part
	// of a structured message to recompose partials ones into only one,
	// we might consider doing the same on structured log messages with
	// the framer.
	if input.State != message.StateUnstructured {
		fr.outputFn(input, len(input.GetContent()))
		fr.frames.Inc()
		return
	}

	// buffer is laid out as follows:
	//
	//                  /------from inBuf------\
	// xxxxxxxxFFFFFFFFFFFFFFffffffffFFFFFFFffff
	//  framed ^   seen ^
	//
	// Here "xx" is data that has already been sent to outputFn; and the "ff" and "FF" are
	// as-yet un-recognized frames of data.  The `seen` offset indicates where the matcher
	// left off in the last call to Process.

	framed := fr.bytesFramed
	seen := fr.buffer.Len()
	fr.buffer.Write(input.GetContent())
	end := fr.buffer.Len()
	contentLenLimit := fr.contentLenLimit

	for {
		if framed == end {
			break
		}
		buf := fr.buffer.Bytes()[framed:]

		content, rawDataLen := fr.matcher.FindFrame(buf, seen-framed)
		if content == nil {
			// if the matcher was asked to match more than contentLenLimit,
			// chop off contentLenLimit raw bytes and output them
			if len(buf) >= contentLenLimit {
				content, rawDataLen = buf[:contentLenLimit], contentLenLimit
				input.ParsingExtra.IsTruncated = true
			} else {
				// matcher didn't find a frame, so leave the remainder in
				// buffer
				break
			}
		}

		// copy the data so that we can reuse fr.buffer (`content` is a slice
		// of `fr.buffer`)
		owned := make([]byte, len(content))
		copy(owned, content)

		c := &message.Message{
			MessageContent: message.MessageContent{
				State: message.StateUnstructured,
			},
			MessageMetadata: message.MessageMetadata{
				Origin:             input.Origin,
				Status:             input.Status,
				IngestionTimestamp: input.IngestionTimestamp,
				ParsingExtra:       input.ParsingExtra,
				ServerlessExtra:    input.ServerlessExtra,
			},
		}
		c.SetContent(owned)

		fr.outputFn(c, rawDataLen)
		fr.frames.Inc()
		framed += rawDataLen
		seen = framed
	}

	fr.bytesFramed = framed
	fr.normalizeBuffer()
}

// normalizeBuffer makes the buffer ready for new data, while attempting to
// minimize copying of data.
func (fr *Framer) normalizeBuffer() {
	framed := fr.bytesFramed

	// if the buffer is completely framed (a common event), reset to the
	// beginning
	if framed == fr.buffer.Len() {
		fr.buffer.Reset()
		fr.bytesFramed = 0
		return
	}

	// if more than half of the buffer is framed, move the remaining bytes
	// to the beginning of the buffer to re-use that space
	if framed*2 > fr.buffer.Len() {
		buf := fr.buffer.Bytes()
		unframed := buf[framed:]
		copy(buf[:len(unframed)], unframed)
		fr.buffer.Truncate(len(unframed))
		fr.bytesFramed = 0
	}
}

// reset resets the framer to begin framing a new stream, for testing.
func (fr *Framer) reset() {
	fr.buffer.Reset()
	fr.bytesFramed = 0
}
