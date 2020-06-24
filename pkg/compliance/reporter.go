// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package compliance

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Reporter defines an interface for reporting rule events
type Reporter interface {
	Report(event *RuleEvent)
}

type reporter struct {
	logSource *config.LogSource
	logChan   chan *message.Message
}

// NewReporter returns an instance of Reporter
func NewReporter(logSource *config.LogSource, logChan chan *message.Message) Reporter {
	return &reporter{
		logSource: logSource,
		logChan:   logChan,
	}
}

func (r *reporter) Report(event *RuleEvent) {
	buf, err := json.Marshal(event)
	if err != nil {
		log.Errorf("Failed to serialize rule event for rule %s", event.RuleID)
		return
	}

	msg := message.NewMessageWithSource(buf, message.StatusInfo, r.logSource)

	r.logChan <- msg
}
