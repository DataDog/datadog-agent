// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerfile

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestDockerFileFormat(t *testing.T) {
	var (
		msg parsers.Message
		err error
	)

	parser := New()

	msg, err = parser.Parse([]byte(`{"log":"a message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`))
	assert.Nil(t, err)
	assert.True(t, msg.IsPartial)
	assert.Equal(t, []byte("a message"), msg.Content)
	assert.Equal(t, message.StatusError, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852911Z", msg.Timestamp)

	msg, err = parser.Parse([]byte(`{"log":"a second message","stream":"stdout","time":"2019-06-06T16:35:55.930852912Z"}`))
	assert.Nil(t, err)
	assert.True(t, msg.IsPartial)
	assert.Equal(t, []byte("a second message"), msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852912Z", msg.Timestamp)

	msg, err = parser.Parse([]byte(`{"log":"a third message\n","stream":"stdout","time":"2019-06-06T16:35:55.930852913Z"}`))
	assert.Nil(t, err)
	assert.False(t, msg.IsPartial)
	assert.Equal(t, []byte("a third message"), msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852913Z", msg.Timestamp)

	msg, err = parser.Parse([]byte("a wrong message"))
	assert.NotNil(t, err)
	assert.Equal(t, []byte("a wrong message"), msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)

	msg, err = parser.Parse([]byte(`{"log":"","stream":"stdout","time":"2019-06-06T16:35:55.930852914Z"}`))
	assert.Nil(t, err)
	assert.False(t, msg.IsPartial)
	assert.Equal(t, []byte(""), msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852914Z", msg.Timestamp)

	msg, err = parser.Parse([]byte(`{"log":"\n","stream":"stdout","time":"2019-06-06T16:35:55.930852915Z"}`))
	assert.Nil(t, err)
	assert.False(t, msg.IsPartial)
	assert.Equal(t, []byte(""), msg.Content)
	assert.Equal(t, message.StatusInfo, msg.Status)
	assert.Equal(t, "2019-06-06T16:35:55.930852915Z", msg.Timestamp)
}
