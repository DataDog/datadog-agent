// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// serviceEnvVar is the environment variable of the service tag (this is used only for the serverless agent)
const serviceEnvVar = "DD_SERVICE"

// Tailer consumes and processes a channel of strings, and sends them to a
// stream of log messages.
//
// This tailer attaches the tags from source.Config.ChannelTags to each
// message, in addition to the origin tags and tags in source.Config.Tags.
type Tailer struct {
	source     *config.LogSource
	inputChan  chan *config.ChannelMessage
	outputChan chan *message.Message
	done       chan interface{}
}

// NewTailer returns a new Tailer
func NewTailer(source *config.LogSource, inputChan chan *config.ChannelMessage, outputChan chan *message.Message) *Tailer {
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
		// tags already contains t.source.Config.Tags
		tags := origin.Tags()

		origin.SetService(computeServiceName(logline.Lambda, os.Getenv(serviceEnvVar)))

		t.source.Config.ChannelTagsMutex.Lock()
		// while access to this field is controlled by the mutex, the slice it
		// points to cannot be modified after it is set, so it's safe to access
		// after releasing the mutex.
		channelTags := t.source.Config.ChannelTags
		t.source.Config.ChannelTagsMutex.Unlock()

		if len(channelTags) > 0 {
			tags = append(tags, channelTags...)
		}
		origin.SetTags(tags)
		if logline.Lambda != nil {
			t.outputChan <- message.NewMessageFromLambda(logline.Content, origin, message.StatusInfo, logline.Timestamp, logline.Lambda.ARN, logline.Lambda.RequestID, time.Now().UnixNano())
		} else {
			t.outputChan <- message.NewMessage(logline.Content, origin, message.StatusInfo, time.Now().UnixNano())
		}
	}
}

func computeServiceName(lambdaConfig *config.Lambda, serviceName string) string {
	if lambdaConfig == nil {
		return "agent"
	}
	if len(serviceName) > 0 {
		return strings.ToLower(serviceName)
	}
	return ""
}
