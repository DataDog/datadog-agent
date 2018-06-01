// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package eventlog

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Config is a event log tailer configuration
type Config struct {
	ChannelPath string
	Query       string
}

// eventContext links go and c
type eventContext struct {
	id int
}

// Tailer collects logs from event log.
type Tailer struct {
	source     *config.LogSource
	config     *Config
	outputChan chan message.Message
	stop       chan struct{}
	done       chan struct{}

	context *eventContext
}

// NewTailer returns a new tailer.
func NewTailer(source *config.LogSource, config *Config, outputChan chan message.Message) *Tailer {
	return &Tailer{
		source:     source,
		config:     config,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Identifier returns a string that uniquely identifies a source
func Identifier(channelPath, query string) string {
	return fmt.Sprintf("eventlog:%s;%s", channelPath, query)
}

func (t *Tailer) Identifier() string {
	return Identifier(t.config.ChannelPath, t.config.Query)
}
