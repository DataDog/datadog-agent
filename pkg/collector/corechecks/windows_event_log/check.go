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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	winevtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	evtsession "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"

	"golang.org/x/sys/windows"
)

const (
	// CheckName is the name of the check
	CheckName = "win32_event_log"
)

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

// Run updates sender stats, restarts the subscription if it failed, and saves the bookmark.
// The main event collection logic runs continuously in the background, not during Run().
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	// Necessary for check stats to be calculated (number of events collected, etc)
	// Since events are collected in the background, this will update stats with the
	// count of events collected since the last Run() call.
	defer sender.Commit()

	// Start/Restart the subscription if it is not running
	if !c.sub.Running() {
		// starts the event collection in the background.
		err := c.startSubscription()
		if err != nil {
			err = fmt.Errorf("subscription is not running, failed to start: %w", err)
			if c.sub.Error() != nil {
				err = fmt.Errorf("%w, last stop reason: %w", err, c.sub.Error())
			}
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
		c.Warnf("error saving bookmark: %v", err)
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
			c.Warnf("error saving bookmark: %v", err)
		}
	}()

	// Fetch new events
	for {
		select {
		case <-c.fetchEventsLoopStop:
			log.Debug("stopping subscription")
			return
		case events, ok := <-c.sub.GetEvents():
			if !ok {
				// The channel is closed, this indicates an error or that sub.Stop() was called
				// Use sub.Error() to get the error, if any.
				err := c.sub.Error()
				if err != nil {
					log.Errorf("event subscription stopped with error: %v", err)
				} else {
					log.Debug("event subscription stopped")
				}
				return
			}
			for _, event := range events {
				// Submit Datadog Event
				c.submitEvent(sender, event)

				// bookmarkSaver manages whether or not to save/persist the bookmark
				err := c.bookmarkSaver.updateBookmark(event)
				if err != nil {
					c.Warnf("%v", err)
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
	c.eventPriority, err = getEventPriorityFromOption(c.config.instance.EventPriority)
	if err != nil {
		return fmt.Errorf("invalid instance config `event_priority`: %w", err)
	}
	if isaffirmative(c.config.instance.LegacyMode) && isaffirmative(c.config.instance.LegacyModeV2) {
		return fmt.Errorf("legacy_mode and legacy_mode_v2 are both true. Each instance must set a single mode to true")
	}
	if isaffirmative(c.config.instance.LegacyMode) {
		// wrap ErrSkipCheckInstance for graceful skipping
		return fmt.Errorf("%w: unsupported configuration: legacy_mode: true", check.ErrSkipCheckInstance)
	}
	if isaffirmative(c.config.instance.LegacyModeV2) {
		// wrap ErrSkipCheckInstance for graceful skipping
		return fmt.Errorf("%w: unsupported configuration: legacy_mode_v2: true", check.ErrSkipCheckInstance)
	}
	if _, isSet := c.config.instance.Timeout.Get(); isSet {
		// timeout option is deprecated. Now that the subscription runs in the background in a select
		// style, a timeout on the "wait for events" operation is no longer applicable.
		c.Warn("instance config `timeout` is deprecated. It is no longer used by the check and can be removed.")
	}
	if val, isSet := c.config.instance.ChannelPath.Get(); !isSet || len(val) == 0 {
		return fmt.Errorf("instance config `path` must be provided and not be empty")
	}
	if val, isSet := c.config.instance.Query.Get(); !isSet || len(val) == 0 {
		// Query should always be set by this point, but might be ""
		return fmt.Errorf("instance config `query` if provided must not be empty")
	}
	startMode, isSet := c.config.instance.Start.Get()
	if !isSet || (startMode != "now" && startMode != "oldest") {
		return fmt.Errorf("invalid instance config `start`: '%s'", startMode)
	}
	_, err = evtRPCFlagsFromOption(c.config.instance.AuthType)
	if err != nil {
		return fmt.Errorf("invalid instance config `auth_type`: %w", err)
	}

	if val, isSet := c.config.instance.IncludedMessages.Get(); isSet {
		c.includedMessages, err = compileRegexPatterns(val)
		if err != nil {
			return fmt.Errorf("invalid instance config `included_messages`: %w", err)
		}
	}

	if val, isSet := c.config.instance.ExcludedMessages.Get(); isSet {
		c.excludedMessages, err = compileRegexPatterns(val)
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

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(func() check.Check {
		return &Check{
			CheckBase: core.NewCheckBase(CheckName),
			evtapi:    winevtapi.New(),
		}
	})
}
