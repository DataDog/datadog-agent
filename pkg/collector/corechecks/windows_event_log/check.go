// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	agentCheck "github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"

	"golang.org/x/sys/windows"
)

const checkName = "win32_event_log"

// The lower cased version of the `API SOURCE ATTRIBUTE` column from the table located here:
// https://docs.datadoghq.com/integrations/faq/list-of-api-source-attribute-value/
const sourceTypeName = "event viewer"

// Check defines a check that reads the Windows Event Log and submits Events
type Check struct {
	// check
	core.CheckBase
	config *Config

	fetchEventsLoopWaiter sync.WaitGroup
	fetchEventsLoopStop   chan struct{}

	includedMessages []*regexp.Regexp
	excludedMessages []*regexp.Regexp

	// event metrics
	eventPriority agentEvent.EventPriority

	// event log
	session             evtsession.Session
	sub                 evtsubscribe.PullSubscription
	evtapi              evtapi.API
	systemRenderContext evtapi.EventRenderContextHandle
	userRenderContext   evtapi.EventRenderContextHandle
	bookmarkSaver       *bookmarkSaver
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	// Necessary for check stats to be calculated (number of events collected, etc)
	defer sender.Commit()

	if !c.sub.Running() {
		err := c.startSubscription()
		if err != nil {
			return err
		}
	}

	// save/persist bookmark on each check run
	// check.Cancel() may not be called (like in agent check command), which means
	// if the events read are less than bookmark_frequency then a bookmark won't
	// be saved before the process exits. saving here, too, gives us a good time periodic
	// save/persist in addition to the count periodic bookmark_frequency option.
	err = c.bookmarkSaver.saveLastBookmark()
	if err != nil {
		c.Warnf(err.Error())
	}

	return nil
}

func (c *Check) fetchEventsLoop(sender sender.Sender) {
	defer c.fetchEventsLoopWaiter.Done()

	// Always stop the subscription when the loop ends.
	// The check will start the subscription and this loop again next time it runs.
	defer c.sub.Stop()

	// Save the bookmark at the end of the loop, regardless of the bookmarkFrequency
	defer func() {
		err := c.bookmarkSaver.saveLastBookmark()
		if err != nil {
			c.Warnf(err.Error())
		}
	}()

	var addBookmarkToSubOnce sync.Once

	// Fetch new events
	for {
		select {
		case <-c.fetchEventsLoopStop:
			return
		case events, ok := <-c.sub.GetEvents():
			if !ok {
				// The channel is closed, this indicates an error or that sub.Stop() was called
				// Use sub.Error() to get the error, if any.
				err := c.sub.Error()
				if err != nil {
					log.Errorf("event subscription stopped with error: %v", err)
				}
				return
			}
			for _, event := range events {
				// Submit Datadog Event
				c.submitEvent(sender, event)

				// bookmarkSaver manages whether or not to save/persist the bookmark
				err := c.bookmarkSaver.updateBookmark(event)
				if err != nil {
					c.Warnf(err.Error())
				} else {
					// If we don't have a bookmark when we create the subscription we have
					// to add it later once we've updated it at least once.
					addBookmarkToSubOnce.Do(func() {
						c.sub.SetBookmark(c.bookmarkSaver.bookmark)
					})
				}

				// Must close event handle when we are done with it
				evtapi.EvtCloseRecord(c.evtapi, event.EventRecordHandle)
			}
		}
	}
}

func (c *Check) submitEvent(sender sender.Sender, event *evtapi.EventRecord) {
	// Base event
	ddevent := agentEvent.Event{
		Priority:       c.eventPriority,
		SourceTypeName: sourceTypeName,
		Tags:           []string{},
	}

	// Render Windows event values into the DD event
	err := c.renderEventValues(event, &ddevent)
	if err != nil {
		log.Error(err)
	}
	// If the event has rendered text, check it against the regexp patterns to see if
	// we should send the event or not
	if len(ddevent.Text) > 0 {
		if !c.includeMessage(ddevent.Text) {
			return
		}
	}

	// submit
	sender.Event(ddevent)
}

func (c *Check) bookmarkPersistentCacheKey() string {
	return fmt.Sprintf("%s_%s", c.ID(), "bookmark")
}

// Configure processes the configuration for the check
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	// common CoreCheck requirements
	// This check supports multiple instances, BuildID must be called before CommonConfigure
	c.BuildID(integrationConfigDigest, data, initConfig)
	err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Add the possibly configured service as a tag for this check
	s, err := c.GetSender()
	if err != nil {
		log.Errorf("failed to retrieve a sender for check %s: %s", string(c.ID()), err)
		return err
	}
	s.FinalizeCheckServiceTag()

	// process configuration
	c.config, err = unmarshalConfig(data, initConfig)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}
	err = c.validateConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	// Create the event subscription
	err = c.initSubscription()
	if err != nil {
		return fmt.Errorf("failed to initialize event subscription: %w", err)
	}

	// subscription will be started on first check run.

	return nil
}

func (c *Check) validateConfig() error {
	var err error
	c.eventPriority, err = agentEvent.GetEventPriorityFromString(*c.config.instance.EventPriority)
	if err != nil {
		return fmt.Errorf("invalid instance config `event_priority`: %w", err)
	}
	if *c.config.instance.LegacyMode && *c.config.instance.LegacyModeV2 {
		return fmt.Errorf("legacy_mode and legacy_mode_v2 are both true. Each instance must set a single mode to true")
	}
	if *c.config.instance.LegacyMode {
		// wrap ErrSkipCheckInstance for graceful skipping
		return fmt.Errorf("%w: unsupported configuration: legacy_mode: true", agentCheck.ErrSkipCheckInstance)
	}
	if *c.config.instance.LegacyModeV2 {
		// wrap ErrSkipCheckInstance for graceful skipping
		return fmt.Errorf("%w: unsupported configuration: legacy_mode_v2: true", agentCheck.ErrSkipCheckInstance)
	}
	if c.config.instance.Timeout != nil {
		// timeout option is deprecated. Now that the subscription runs in the background in a select
		// style, a timeout on the "wait for events" operation is no longer applicable.
		c.Warn("instance config `timeout` is deprecated. It is no longer used by the check and can be removed.")
	}
	if len(*c.config.instance.ChannelPath) == 0 {
		return fmt.Errorf("instance config `path` must be provided and not be empty")
	}
	if len(*c.config.instance.Query) == 0 {
		return fmt.Errorf("instance config `query` if provided must not be empty")
	}
	if *c.config.instance.Start != "now" && *c.config.instance.Start != "oldest" {
		return fmt.Errorf("invalid instance config `start`: '%s'", *c.config.instance.Start)
	}
	_, err = evtRPCFlagsFromString(*c.config.instance.AuthType)
	if err != nil {
		return fmt.Errorf("invalid instance config `auth_type`: %w", err)
	}

	if c.config.instance.IncludedMessages != nil {
		c.includedMessages, err = compileRegexPatterns(c.config.instance.IncludedMessages)
		if err != nil {
			return fmt.Errorf("invalid instance config `included_messages`: %w", err)
		}
	}

	if c.config.instance.ExcludedMessages != nil {
		c.excludedMessages, err = compileRegexPatterns(c.config.instance.ExcludedMessages)
		if err != nil {
			return fmt.Errorf("invalid instance config `excluded_messages`: %w", err)
		}
	}

	return nil
}

// Cancel stops the check and releases resources
func (c *Check) Cancel() {
	// stop background loop and wait for it to finish
	c.stopSubscription()

	if c.session != nil {
		c.session.Close()
	}

	if c.bookmarkSaver != nil && c.bookmarkSaver.bookmark != nil {
		c.bookmarkSaver.bookmark.Close()
	}

	if c.systemRenderContext != evtapi.EventRenderContextHandle(0) {
		c.evtapi.EvtClose(windows.Handle(c.systemRenderContext))
	}
}

func checkFactory() agentCheck.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
		evtapi:    winevtapi.New(),
	}
}

func init() {
	core.RegisterCheck(checkName, checkFactory)
}
