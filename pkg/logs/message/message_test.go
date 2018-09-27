// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package message

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/severity"
	"github.com/stretchr/testify/assert"
)

func TestMessage(t *testing.T) {

	message := Message{Content: []byte("hello")}
	assert.Equal(t, "hello", string(message.Content))

	message.Content = []byte("world")
	assert.Equal(t, "world", string(message.Content))
	assert.Equal(t, severity.StatusInfo, message.GetStatus())

}
