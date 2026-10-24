// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	pipelinemock "github.com/DataDog/datadog-agent/comp/logs-library/pipeline/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestDrainIncludesJustAddedSource drains right after the launcher receives a
// source, before the run goroutine may have registered its tailer.
func TestDrainIncludesJustAddedSource(t *testing.T) {
	sourceProvider := launchers.NewMockSourceProvider()
	pipelineProvider := pipelinemock.NewMockProvider()
	launcher := NewLauncher()
	launcher.Start(sourceProvider, pipelineProvider, nil, nil)
	defer launcher.Stop()

	logs := make(chan *config.ChannelMessage, 1)
	logs <- &config.ChannelMessage{Content: []byte("line")}
	sourceProvider.SourceChan <- sources.NewLogSource("test", &config.LogsConfig{
		Type:    config.StringChannelType,
		Channel: logs,
	})

	forwarded := make(chan *message.Message, 1)
	go func() {
		forwarded <- <-pipelineProvider.NextPipelineChan()
	}()

	launcher.Drain(context.Background())

	select {
	case _, open := <-logs:
		assert.False(t, open, "Drain must close the source channel")
	default:
		t.Fatal("Drain did not flush the tailer of the source")
	}
	assert.Equal(t, "line", string((<-forwarded).GetContent()))
}
