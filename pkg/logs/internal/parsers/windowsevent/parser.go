// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package windowsevent

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type windowseventParser struct {
}

func New() parsers.Parser {
	return &windowseventParser{}
}

func (p *windowseventParser) Parse(msg *message.Message) (*message.Message, error) {
	return msg, nil
}

func (p *windowseventParser) SupportsPartialLine() bool {
	return false
}
