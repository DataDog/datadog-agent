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
	assert.NotNil(t, err)
	assert.Equal(t, []byte(`not valid json`), msg.GetContent())

	// extract ddtags
	logMessage.SetContent([]byte(`{"log":"third message","ddtags":"foo:bar,env:prod"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"third message"}`), msg.GetContent())
	assert.Equal(t, []string{"foo:bar", "env:prod"}, msg.ParsingExtra.Tags)

	// empty tags
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"fourth message","ddtags":""}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"fourth message"}`), msg.GetContent())
	assert.Equal(t, []string{}, msg.ParsingExtra.Tags)

	// Malformed ddtags (non-string)
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"fifth message","ddtags":12345}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"fifth message","ddtags":12345}`), msg.GetContent())
	assert.Equal(t, []string{}, msg.ParsingExtra.Tags)

	// Leading and trailing whitespace in ddtags
	logMessage.SetContent([]byte(`{"log":"sixth message","ddtags":"  foo:bar , env:prod  "}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"sixth message"}`), msg.GetContent())
	assert.Equal(t, []string{"foo:bar", "env:prod"}, msg.ParsingExtra.Tags)

	// Missing closing quotation
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"seventh message}`))
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte(`{"log":"seventh message}`), msg.GetContent())
	assert.Equal(t, []string{}, msg.ParsingExtra.Tags)

	// Empty tag in tags list
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"foo bar", "ddtags":"env:prod,,"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"foo bar"}`), msg.GetContent())
	assert.Equal(t, []string{"env:prod"}, msg.ParsingExtra.Tags)

	// Test with extra commas
	logMessage = message.NewMessage([]byte(`{"log":"extra comma",}`), nil, "", 0)
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte(`{"log":"extra comma",}`), msg.GetContent())

	// Test with single quotes instead of double quotes
	logMessage = message.NewMessage([]byte(`{'log':'single quotes'}`), nil, "", 0)
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte(`{'log':'single quotes'}`), msg.GetContent())

	// Test with incorrect nesting
	logMessage = message.NewMessage([]byte(`{"log": {"nested": "value"}`), nil, "", 0)
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte(`{"log": {"nested": "value"}`), msg.GetContent())

	// Special characters in ddtags
	msg.ParsingExtra.Tags = []string{}
	logMessage.SetContent([]byte(`{"log":"foo bar","ddtags":"foo:bar,env:prød,sp@ce:ch@r"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`{"log":"foo bar"}`), msg.GetContent())
	assert.Equal(t, []string{"foo:bar", "env:prød", "sp@ce:ch@r"}, msg.ParsingExtra.Tags)
}
