// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// NoopParser is the default parser and does nothing
var NoopParser *noopParser

// Parser parse messages
type Parser interface {
	Parse([]byte) (*message.Message, error)
	Unwrap([]byte) ([]byte, error)
}

type noopParser struct {
	Parser
}

// Parse does nothing for NoopParser
func (p *noopParser) Parse(msg []byte) (*message.Message, error) {
	return &message.Message{Content: msg}, nil
}

// Unwrap does nothing for NoopParser
func (p *noopParser) Unwrap(msg []byte) ([]byte, error) {
	return msg, nil
}
