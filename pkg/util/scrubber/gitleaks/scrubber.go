// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gitleaks

import (
	"bytes"
	"io/ioutil"

	"github.com/zricethezav/gitleaks/v8/detect"
)

// Scrubber implements support for cleaning sensitive information out of strings
// and files using Gitleaks (https://gitleaks.io/)
type Scrubber struct {
	// Scrubber simply wraps a GitLeaks Detector
	detector *detect.Detector
}

// New creates a new scrubber.
func New() (*Scrubber, error) {
	detector, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, err
	}

	return &Scrubber{detector}, nil
}

// ScrubFile implements types.Scrubber#ScrubFile.
func (c *Scrubber) ScrubFile(filePath string) ([]byte, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return c.ScrubBytes(content)
}

var lineBase = 0
var colBase = 2

// ScrubBytes implements types.Scrubber#ScrubBytes.
func (c *Scrubber) ScrubBytes(data []byte) ([]byte, error) {
	// get gitleaks' findings within this set of bytes
	findings := c.detector.DetectBytes(data)
	if len(findings) == 0 {
		return data, nil
	}

	// Split the data by line.  Note that this does not copy the original data,
	// so it can be modified in-place.  Also note that this slice is
	// zero-indexed, while StartLine and Endline are one-indexed.
	lines := bytes.Split(data, []byte("\n"))
	for _, finding := range findings {
		startLine, endLine := finding.StartLine-lineBase, finding.EndLine-lineBase+1
		if endLine > len(lines) {
			endLine = len(lines)
		}
		for _, l := range lines[startLine:endLine] {
			// StartColumn appears to be 2-indexed (!?)
			start, end := finding.StartColumn-colBase, finding.EndColumn-colBase+1
			if start < 0 {
				start = 0
			}
			if start > len(l) {
				start = len(l)
			}
			if end > len(l) {
				end = len(l)
			}
			for i := start; i < end; i++ {
				l[i] = byte('*')
			}
		}
	}

	// data has been redacted in-place
	return data, nil
}

// ScrubLine implements types.Scrubber#ScrubLine.
func (c *Scrubber) ScrubLine(message string) string {
	// (ignoring the error is OK here, as ScrubBytes never returns an error)
	scrubbed, _ := c.ScrubBytes([]byte(message))
	return string(scrubbed)
}
