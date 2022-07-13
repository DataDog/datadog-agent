// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package multi

import (
	"io/ioutil"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Scrubber impelements a scrubber.Scrubber that passes its input through multiple
// other scrubbers, in sequence
type Scrubber struct {
	children []scrubber.Scrubber
}

// NewScrubber creates a new comment-stripping scrubber.
func NewScrubber(children []scrubber.Scrubber) *Scrubber {
	return &Scrubber{children}
}

// ScrubFile implements scrubber.Scrubber#ScrubFile.
func (c *Scrubber) ScrubFile(filePath string) ([]byte, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return c.ScrubBytes(data)
}

// ScrubBytes implements scrubber.Scrubber#ScrubBytes.
func (c *Scrubber) ScrubBytes(data []byte) ([]byte, error) {
	for _, child := range c.children {
		var err error
		data, err = child.ScrubBytes(data)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// ScrubLine implements scrubber.Scrubber#ScrubLine.
func (c *Scrubber) ScrubLine(message string) string {
	for _, child := range c.children {
		message = child.ScrubLine(message)
	}
	return message
}
