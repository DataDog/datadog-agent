// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// chanSource is the LogSource installed by this scheduler.
var chanSource *config.LogSource

// Scheduler is a logs-agent Scheduler that only manages one source, connected to
// the serverless logs channel.
type Scheduler struct {
	logsChan  chan *config.ChannelMessage
	extraTags []string
}

var _ schedulers.Scheduler = &Scheduler{}

// NewScheduler creates a new Scheduler.
func NewScheduler(logsChan chan *config.ChannelMessage, extraTags []string) schedulers.Scheduler {
	return &Scheduler{logsChan, extraTags}
}

// Start implements schedulers.Scheduler#Start.
func (s *Scheduler) Start(sourceMgr schedulers.SourceManager) {
	log.Debug("Adding AWS Logs collection source")

	chanSource = config.NewLogSource("AWS Logs", &config.LogsConfig{
		Type:    config.StringChannelType,
		Source:  "lambda", // TODO(remy): do we want this to be configurable at some point?
		Tags:    s.extraTags,
		Channel: s.logsChan,
	})
	sourceMgr.AddSource(chanSource)
}

// Stop implements schedulers.Scheduler#Stop.
func (s *Scheduler) Stop() {}

// UpdateLogsTags updates the tags used for this source.
func UpdateLogsTags(tags []string) {
	if chanSource != nil {
		// NOTE(dustin): this is unsafe and susceptible to "write tearing", if
		// the logs agent happens to be reading from this value at the same
		// time as we write to it.
		chanSource.Config.Tags = tags
	}
}
