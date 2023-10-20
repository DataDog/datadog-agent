// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noop

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/stretchr/testify/assert"
)

func TestNoopParserHandleMessages(t *testing.T) {
	parser := New()
	logMessage := message.NewMessage([]byte("Foo"), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, logMessage.ParsingExtra.IsPartial)
	assert.Equal(t, logMessage.GetContent(), msg.GetContent())
}
