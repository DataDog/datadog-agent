// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// XXX(remy): all unit tests related to structured/unstructured, the state, etc.

func TestMessage(t *testing.T) {

	message := NewMessage([]byte("hello"), nil, "", 0)
	assert.Equal(t, "hello", string(message.GetContent()))

	message.SetContent([]byte("world"))
	assert.Equal(t, "world", string(message.GetContent()))
	assert.Equal(t, StatusInfo, message.GetStatus())

}

func TestGetHostnameLambda(t *testing.T) {
	message := Message{
		ServerlessExtra: ServerlessExtra{
			Lambda: &Lambda{
				ARN: "testHostName",
			},
		},
	}
	assert.Equal(t, "testHostName", message.GetHostname())
}

func TestGetHostname(t *testing.T) {
	t.Setenv("DD_HOSTNAME", "testHostnameFromEnvVar")
	message := NewMessage([]byte("hello"), nil, "", 0)
	assert.Equal(t, "testHostnameFromEnvVar", message.GetHostname())
}
