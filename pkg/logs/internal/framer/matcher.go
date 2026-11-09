// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

// FrameMatcher finds frames in a buffer
type FrameMatcher interface {
	// Find a frame in a prefix of buf, and return the slice containing the content
	// of that frame, together with the total number of bytes in that frame, and whether
	// the frame was truncated due to exceeding the content length limit.
	// Return `nil, 0, false` when no complete frame is present in buf.
	//
	// "never the last": when a single logical line is split into multiple
	// frames because it exceeds the content length limit, every emitted segment
	// except the final one must report wasTruncated=true. The final segment
	// reports false — it carries no trailing "...TRUNCATED..." marker because no
	// content continues past it, and the downstream handler prepends the leading
	// marker via its carry-over state. A frame that fits whole is never truncated.
	//
	// Return `nil, N, false` with N > 0 to consume N leading bytes that do not
	// form a frame (e.g. a stray delimiter or a zero-length declared frame).
	// The framer advances past those bytes without emitting a frame, so the
	// matcher never has to produce an empty frame to skip data.
	//
	// The `seen` argument is the length of `buf` last time this function was called,
	// and can be used to avoid repeating work when looking for a frame terminator.
	FindFrame(buf []byte, seen int) (content []byte, rawDataLen int, wasTruncated bool)

	// FlushFrame is called at end-of-stream with any unframed remainder in buf.
	// If the remainder represents a valid frame that should be emitted, return
	// the content and raw byte length. Return nil, 0 to discard the remainder.
	//
	// A flushed frame is always the terminal segment of its logical line — the
	// Framer keeps the buffered remainder under contentLenLimit, so there is
	// nothing left to continue into — so FlushFrame never reports truncation.
	// The "...TRUNCATED..." continuation marker on a flushed tail (if its frame
	// was split during Process) is added by the downstream line handler via its
	// carry-over state, not by this flag. See the "never the last" contract:
	// every split segment except the final one is flagged truncated.
	FlushFrame(buf []byte) (content []byte, rawDataLen int)
}
