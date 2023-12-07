// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package channel

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler is a logs-agent Scheduler that only manages one source, connected
// to a channel. Messages sent to the channel will be directed to a logs pipeline.
//
// After the scheduler is started, the tags assigned to the log source can be
// updated with SetLogsTags.
type Scheduler struct {
	// sourceName is the name of the LogSource the scheduler creates
	sourceName string

	// source is the Source of the LogsConfig the scheduler creates
	source string

	// logsChan is the channel carrying messages to be sent to the pipeline
	logsChan chan *config.ChannelMessage

	// logSource is the source currently managed by the scheduler
	logSource *sources.LogSource

	// sourceMgr is the schedulers.SourceManager used to add/remove sources
	sourceMgr schedulers.SourceManager
}

var _ schedulers.Scheduler = &Scheduler{}

// NewScheduler creates a new Scheduler.
func NewScheduler(sourceName, source string, logsChan chan *config.ChannelMessage) *Scheduler {
	return &Scheduler{
		sourceName: sourceName,
		source:     source,
		logsChan:   logsChan,
	}
}

// Start implements schedulers.Scheduler#Start.
func (s *Scheduler) Start(sourceMgr schedulers.SourceManager) {
	s.sourceMgr = sourceMgr

	log.Debugf("Adding %s Log Source", s.sourceName)
	s.setSource()
}

// setSource creates a source based on the current configuration of this
// scheduler, and adds it to the logs agent.  If there was a previous source,
// then that is removed first.  The sources share the same channel, so no
// messages will be lost in the translation.
func (s *Scheduler) setSource() {
	if s.logSource != nil {
		s.sourceMgr.RemoveSource(s.logSource)
	}

	s.logSource = sources.NewLogSource(s.sourceName, &config.LogsConfig{
		Type:    config.StringChannelType,
		Source:  s.source,
		Channel: s.logsChan,
	})
	s.sourceMgr.AddSource(s.logSource)
}

// Stop implements schedulers.Scheduler#Stop.
func (s *Scheduler) Stop() {}

// SetLogsTags updates the tags attached to channel messages.
//
// This method retains the given tags slice, which must not be modified after this
// call.
func (s *Scheduler) SetLogsTags(tags []string) {
	s.logSource.Config.ChannelTagsMutex.Lock()
	defer s.logSource.Config.ChannelTagsMutex.Unlock()
	s.logSource.Config.ChannelTags = tags
}

// GetLogsTags returns a defensive copy of the used tags
func (s *Scheduler) GetLogsTags() []string {
	s.logSource.Config.ChannelTagsMutex.Lock()
	defer s.logSource.Config.ChannelTagsMutex.Unlock()
	defensiveCopy := make([]string, len(s.logSource.Config.ChannelTags))
	copy(defensiveCopy, s.logSource.Config.ChannelTags)
	return defensiveCopy
}
