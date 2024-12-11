// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package noop implements a parser that simply returns its input unchanged.
package noop

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// New creates a default parser that simply returns lines unchanged as messages
func New() parsers.Parser {
	return &noop{}
}

type noop struct{}

// Parse implements Parser#Parse
func (p *noop) Parse(msg *message.Message) (*message.Message, error) {
	return msg, nil
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *noop) SupportsPartialLine() bool {
	return false
}
