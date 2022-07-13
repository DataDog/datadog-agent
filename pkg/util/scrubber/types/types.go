// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// Scrubber implements support for cleaning sensitive information out of strings
// and files.  Its intended use is to "clean" data before it is logged or
// transmitted to a remote system, so that the meaning of the data remains
// clear without disclosing any sensitive information.
type Scrubber interface {
	// ScrubFile scrubs credentials from file given by pathname
	ScrubFile(filePath string) ([]byte, error)

	// ScrubBytes scrubs credentials from slice of bytes
	ScrubBytes(file []byte) ([]byte, error)

	// ScrubLine scrubs credentials from a single line of text.  It can be safely
	// applied to URLs or to strings containing URLs.  Scrubber implementations may
	// optimize this function by assuming its input does not contain newlines.
	ScrubLine(message string) string
}
