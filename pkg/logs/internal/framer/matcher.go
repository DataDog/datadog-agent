// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

// FrameMatcher finds frames in a buffer
type FrameMatcher interface {
	// Find a frame in a prefix of buf, and return the slice containing the content
	// of that frame, together with the total number of bytes in that frame.  Return
	// `nil, 0` when no complete frame is present in buf.
	//
	// The `seen` argument is the length of `buf` last time this function was called,
	// and can be used to avoid repeating work when looking for a frame terminator.
	FindFrame(buf []byte, seen int) ([]byte, int)
}
