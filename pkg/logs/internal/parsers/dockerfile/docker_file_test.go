// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestDockerFileFormat(t *testing.T) {
	parser := New()

	logMessage := message.NewMessage([]byte(`{"log":"a message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a message"), msg.GetContent())
	assert.Equal(t, message.StatusError, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"a second message","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a second message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"a third message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852913Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte("a third message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte("a wrong message"))
	msg, err = parser.Parse(logMessage)
	assert.NotNil(t, err)
	assert.Equal(t, []byte("a wrong message"), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)

	logMessage.SetContent([]byte(`{"log":"","stream":"stdout","time":"2019-06-06T16:35:55.930852914Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte(""), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", msg.ParsingExtra.Timestamp)

	logMessage.SetContent([]byte(`{"log":"\n","stream":"stdout","time":"2019-06-06T16:35:55.930852915Z"}`))
	msg, err = parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, msg.ParsingExtra.IsPartial)
	assert.Equal(t, []byte(""), msg.GetContent())
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852915Z", msg.ParsingExtra.Timestamp)
}
