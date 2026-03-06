// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestComputeServiceNameOrderOfPrecedent(t *testing.T) {
	assert.Equal(t, "agent", getServiceName())
	t.Setenv("CONTAINER_APP_NAME", "CONTAINER-app-name")
	assert.Equal(t, "container-app-name", getServiceName())
	t.Setenv("K_SERVICE", "CLOUD-run-service-name")
	assert.Equal(t, "cloud-run-service-name", getServiceName())
	t.Setenv("DD_SERVICE", "DD-service-name")
	assert.Equal(t, "dd-service-name", getServiceName())
}

func TestBuildMessage(t *testing.T) {
	logline := &config.ChannelMessage{
		Content:   []byte("bababang"),
		Timestamp: time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC),
		IsError:   false,
	}
	origin := &message.Origin{}
	builtMessage := buildMessage(logline, origin)
	assert.Equal(t, "bababang", string(builtMessage.GetContent()))
	assert.Equal(t, message.StatusInfo, builtMessage.GetStatus())
}

func TestBuildErrorMessage(t *testing.T) {
	logline := &config.ChannelMessage{
		Content:   []byte("bababang"),
		Timestamp: time.Date(2010, 01, 01, 01, 01, 01, 00, time.UTC),
		IsError:   true,
	}
	origin := &message.Origin{}
	builtMessage := buildMessage(logline, origin)
	assert.Equal(t, "bababang", string(builtMessage.GetContent()))
	assert.Equal(t, message.StatusError, builtMessage.GetStatus())
}

func TestTailerStartAndWaitFlush(t *testing.T) {
	inputChan := make(chan *config.ChannelMessage, 10)
	outputChan := make(chan *message.Message, 10)
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:   config.StringChannelType,
		Source: "test-source",
	})

	tailer := NewTailer(source, inputChan, outputChan)
	tailer.Start()

	// Send a message
	inputChan <- &config.ChannelMessage{
		Content: []byte("hello world"),
		IsError: false,
	}

	// Read the output
	select {
	case msg := <-outputChan:
		assert.Equal(t, "hello world", string(msg.GetContent()))
		assert.Equal(t, message.StatusInfo, msg.GetStatus())
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	tailer.WaitFlush()
}

func TestTailerMultipleMessages(t *testing.T) {
	inputChan := make(chan *config.ChannelMessage, 10)
	outputChan := make(chan *message.Message, 10)
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:   config.StringChannelType,
		Source: "test-source",
	})

	tailer := NewTailer(source, inputChan, outputChan)
	tailer.Start()

	inputChan <- &config.ChannelMessage{Content: []byte("msg1"), IsError: false}
	inputChan <- &config.ChannelMessage{Content: []byte("msg2"), IsError: true}

	msg1 := <-outputChan
	assert.Equal(t, "msg1", string(msg1.GetContent()))
	assert.Equal(t, message.StatusInfo, msg1.GetStatus())

	msg2 := <-outputChan
	assert.Equal(t, "msg2", string(msg2.GetContent()))
	assert.Equal(t, message.StatusError, msg2.GetStatus())

	tailer.WaitFlush()
}

func TestTailerWithChannelTags(t *testing.T) {
	inputChan := make(chan *config.ChannelMessage, 10)
	outputChan := make(chan *message.Message, 10)
	source := sources.NewLogSource("test", &config.LogsConfig{
		Type:   config.StringChannelType,
		Source: "test-source",
	})

	// Set channel tags before starting
	source.Config.ChannelTags = []string{"env:prod", "service:web"}

	tailer := NewTailer(source, inputChan, outputChan)
	tailer.Start()

	inputChan <- &config.ChannelMessage{Content: []byte("tagged message")}

	msg := <-outputChan
	assert.Equal(t, "tagged message", string(msg.GetContent()))
	// Origin should have the channel tags attached
	assert.Equal(t, []string{"env:prod", "service:web"}, msg.Origin.Tags(nil))

	tailer.WaitFlush()
}
