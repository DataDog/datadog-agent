// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package comments

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
)

// Scrubber impelements a scrubber.Scrubber that removes comment lines from
// multi-line inputs.
//
// A comment is any line beginning with whitespace and `#`.  Trailing comments
// (lines containing non-whitspace chracters before the `#`) are _not_
// scrubbed.  All lines in the output are newline-terminated.
//
// Single-line inputs (via ScrubLine) are not scrubbed at all.
type Scrubber struct{}

// NewScrubber creates a new comment-stripping scrubber.
func NewScrubber() *Scrubber {
	return &Scrubber{}
}

// ScrubFile implements scrubber.Scrubber#ScrubFile.
func (c *Scrubber) ScrubFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return c.scrubReader(file)
}

// ScrubBytes implements scrubber.Scrubber#ScrubBytes.
func (c *Scrubber) ScrubBytes(file []byte) ([]byte, error) {
	r := bytes.NewReader(file)
	return c.scrubReader(r)
}

var commentRegex = regexp.MustCompile(`^\s*#.*$`)

// scrubReader implements the comment removal
func (c *Scrubber) scrubReader(file io.Reader) ([]byte, error) {
	var cleanedFile []byte

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) {
			cleanedFile = append(cleanedFile, b...)
			cleanedFile = append(cleanedFile, byte('\n'))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return cleanedFile, nil
}

// ScrubLine implements scrubber.Scrubber#ScrubLine.
func (c *Scrubber) ScrubLine(message string) string {
	return message
}
