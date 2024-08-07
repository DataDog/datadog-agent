// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrations

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// TestIntegrationsFile tests the integration parser
func TestIntegrationsFile(t *testing.T) {
	parser := New()
	// Extract nothing, submit the log as is
	logMessage := message.NewMessage([]byte(`{"log":"first message","time":"2019-06-06T16:35:55.930852911Z"}`), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []string(nil), logMessage.ParsingExtra.Tags)
	assert.Equal(t, []byte(`{"log":"first message","time":"2019-06-06T16:35:55.930852911Z"}`), msg.GetContent())

	// Submit the log immediately if it's not valid JSON
	logMessage = message.NewMessage([]byte(`not valid json`), nil, "", 0)
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`not valid json`), msg.GetContent())

	// extract ddtags
	logMessage.SetContent([]byte(`{"log":"second message","ddtags":"foo:bar,env:prod"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"second message"}`), msg.GetContent())
	assert.Equal(t, []string{"foo:bar", "env:prod"}, msg.ParsingExtra.Tags)

	// empty tags
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"second message","ddtags":""}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"second message"}`), msg.GetContent())
	assert.Equal(t, []string{}, msg.ParsingExtra.Tags)
}
