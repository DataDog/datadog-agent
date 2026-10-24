// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"context"
	"testing"
	"testing/synctest"
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

func TestWaitFlushForwardsQueuedLinesAndIsIdempotent(t *testing.T) {
	input := make(chan *config.ChannelMessage, 1)
	output := make(chan *message.Message, 1)
	source := sources.NewLogSource("test", &config.LogsConfig{})
	tailer := NewTailer(source, input, output)
	tailer.Start()

	input <- &config.ChannelMessage{Content: []byte("line")}
	tailer.WaitFlush(context.Background())
	tailer.WaitFlush(context.Background())

	select {
	case msg := <-output:
		assert.Equal(t, "line", string(msg.GetContent()))
	case <-time.After(time.Second):
		t.Fatal("queued line was not forwarded")
	}
}

// TestWaitFlushReturnsWhenContextIsDone blocks the tailer on a full pipeline
// and checks that WaitFlush still returns at the context deadline.
func TestWaitFlushReturnsWhenContextIsDone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := make(chan *config.ChannelMessage, 1)
		output := make(chan *message.Message)
		tailer := NewTailer(sources.NewLogSource("test", &config.LogsConfig{}), input, output)
		tailer.Start()
		input <- &config.ChannelMessage{Content: []byte("line")}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		start := time.Now()
		tailer.WaitFlush(ctx)
		assert.Equal(t, time.Second, time.Since(start))

		// Read the pending line so the tailer goroutine exits before the bubble ends.
		<-output
		tailer.WaitFlush(context.Background())
	})
}

func TestComputeServiceNameContainerAppJob(t *testing.T) {
	t.Setenv("CONTAINER_APP_JOB_NAME", "JOB-name")
	assert.Equal(t, "job-name", getServiceName())
	t.Setenv("CONTAINER_APP_NAME", "APP-name")
	assert.Equal(t, "app-name", getServiceName())
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
