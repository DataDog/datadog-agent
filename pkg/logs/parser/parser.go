// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

// ParsedLine represents a containerd message
type ParsedLine struct {
	Content   []byte
	Severity  string
	Timestamp string
}

// Parser parse messages
type Parser interface {
	Parse([]byte) (ParsedLine, error)
	Unwrap([]byte) ([]byte, error)
}

// NoopParser is the default parser and does nothing
type NoopParser struct{}

// NewNoopParser returns a new NoopParser
func NewNoopParser() *NoopParser {
	return &NoopParser{}
}

// Parse does nothing for NoopParser
func (p *NoopParser) Parse(msg []byte) (ParsedLine, error) {
	return ParsedLine{Content: msg, Severity: ""}, nil
}

// Unwrap does nothing for NoopParser
func (p *NoopParser) Unwrap(msg []byte) ([]byte, error) {
	return msg, nil
}
