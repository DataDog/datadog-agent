// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noop

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoopParserHandleMessages(t *testing.T) {
	parser := New()
	testMsg := []byte("Foo")
	msg, err := parser.Parse(testMsg)
	assert.Nil(t, err)
	assert.False(t, msg.IsPartial)
	assert.Equal(t, testMsg, msg.Content)
}
