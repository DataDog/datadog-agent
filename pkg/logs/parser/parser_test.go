// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopParserHandleMessages(t *testing.T) {
	parser := NewNoopParser()
	testMsg := []byte("Foo")
	msg, err := parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.Equal(t, testMsg, msg.Content)
}
