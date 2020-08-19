// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package parser

// NoopParser is the default parser and does nothing
var NoopParser *noopParser

// Parser parse messages
type Parser interface {
	// It returns 1. raw message, 2. severity, 3. timestamp, 4. partial, 5. error
	Parse([]byte) ([]byte, string, string, bool, error)
	SupportsPartialLine() bool
}

type noopParser struct {
	Parser
}

// Parse does nothing for NoopParser
func (p *noopParser) Parse(msg []byte) ([]byte, string, string, bool, error) {
	return msg, "", "", false, nil
}

func (p *noopParser) SupportsPartialLine() bool {
	return false
}
