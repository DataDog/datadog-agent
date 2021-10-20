// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

// Noop is the default parser and simply returns lines unchanged as messages
var Noop *noop

type noop struct{}

// Parse implements Parser#Parse
func (p *noop) Parse(msg []byte) ([]byte, string, string, bool, error) {
	return msg, "", "", false, nil
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *noop) SupportsPartialLine() bool {
	return false
}
