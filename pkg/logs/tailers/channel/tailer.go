// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package channel

import (
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// serviceEnvVar is the environment variable of the service tag (this is used only for the serverless agent)
const serviceEnvVar = "DD_SERVICE"

// cloudRunServiceName is the environment variable of the service name tag (this is used only for the serverless agent)
const cloudRunServiceName = "K_SERVICE"

// containerAppName is the environment variable of the container app name (this is used only for the serverless agent)
const containerAppName = "CONTAINER_APP_NAME"

// Tailer consumes and processes a channel of strings, and sends them to a
// stream of log messages.
//
// This tailer attaches the tags from source.Config.ChannelTags to each
// message, in addition to the origin tags and tags in source.Config.Tags.
type Tailer struct {
	source     *sources.LogSource
	inputChan  chan *config.ChannelMessage
	outputChan chan message.TimedMessage[*message.Message]
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(source *sources.LogSource, inputChan chan *config.ChannelMessage, outputChan chan message.TimedMessage[*message.Message]) *Tailer {
	return &Tailer{
		source:     source,
		inputChan:  inputChan,
		outputChan: outputChan,
		done:       make(chan interface{}, 1),
	}
}

// Start starts the tailer.
func (t *Tailer) Start() {
	go t.run()
}

// WaitFlush waits for all items in the input channel to be processed.  In the
// process, this closes the input channel.  Any subsequent sends to the channel
// will fail.
func (t *Tailer) WaitFlush() {
	close(t.inputChan)
	<-t.done
}

func (t *Tailer) run() {
	defer func() {
		t.done <- true
	}()

	// Loop terminates when the channel is closed.
	for logline := range t.inputChan {
		origin := message.NewOrigin(t.source)
		origin.SetService(getServiceName())

		t.source.Config.ChannelTagsMutex.Lock()
		// while access to this field is controlled by the mutex, the slice it
		// points to cannot be modified after it is set, so it's safe to access
		// after releasing the mutex.
		channelTags := t.source.Config.ChannelTags
		t.source.Config.ChannelTagsMutex.Unlock()

		// add additional tags (beyond those from t.source.Config.Tags) to the agent
		if len(channelTags) > 0 {
			origin.SetTags(channelTags)
		}

		t.outputChan <- message.NewTimedMessage(buildMessage(logline, origin))
	}
}

func buildMessage(logline *config.ChannelMessage, origin *message.Origin) *message.Message {
	status := message.StatusInfo
	if logline.IsError {
		status = message.StatusError
	}

	if logline.Lambda != nil {
		return message.NewMessageFromLambda(logline.Content, origin, status, logline.Timestamp, logline.Lambda.ARN, logline.Lambda.RequestID, time.Now().UnixNano())
	}
	return message.NewMessage(logline.Content, origin, status, time.Now().UnixNano())
}

func getServiceName() string {
	if service := os.Getenv(serviceEnvVar); service != "" {
		return strings.ToLower(service)
	}
	if cloudRunService := os.Getenv(cloudRunServiceName); cloudRunService != "" {
		return strings.ToLower(cloudRunService)
	}
	if containerAppName := os.Getenv(containerAppName); containerAppName != "" {
		return strings.ToLower(containerAppName)
	}
	return "agent"
}
