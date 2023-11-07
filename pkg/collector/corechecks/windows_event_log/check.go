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
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	agentCheck "github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"

	"golang.org/x/sys/windows"
)

const checkName = "windows_event_log"

// The lower cased version of the `API SOURCE ATTRIBUTE` column from the table located here:
// https://docs.datadoghq.com/integrations/faq/list-of-api-source-attribute-value/
const sourceTypeName = "event viewer"

// Check defines a check that reads the Windows Event Log and submits Events
type Check struct {
	// check
	core.CheckBase
	config *Config

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
	if !c.sub.Running() {
		err := c.sub.Start()
		if err != nil {
			return fmt.Errorf("failed to start event subscription: %v", err)
		}
	}

	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	err = c.fetchEventsLoop(sender)
	if err != nil {
		// An error occurred fetching events, stop the subscription
		c.sub.Stop()
		return fmt.Errorf("failed to fetch events: %v", err)
	}

	return nil
}

func (c *Check) fetchEventsLoop(sender sender.Sender) error {
	// Save the bookmark at the end of the check, regardless of the bookmarkFrequency
	defer func() {
		err := c.bookmarkSaver.saveLastBookmark()
		if err != nil {
			c.Warnf(err.Error())
		}
	}()
	// Fetch new events
	for {
		stop, err := c.fetchEvents(sender)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
}

func (c *Check) fetchEvents(sender sender.Sender) (bool, error) {
	// Use time.NewTimer instead of time.After so we don't leak a background task each
	// loop iteration. see https://pkg.go.dev/time#After
	timeout := time.NewTimer(time.Duration(*c.config.instance.Timeout) * time.Second)
	defer timeout.Stop()

	select {
	case <-timeout.C:
		// Waited for timeout, did not receive any new events, end the check
		return true, nil
	case events, ok := <-c.sub.GetEvents():
		if !ok {
			// The channel is closed, this indicates an error or that sub.Stop() was called
			// Use sub.Error() to get the error, if any.
			err := c.sub.Error()
			if err != nil {
				return true, err
			}
			return true, nil
		}
		for _, event := range events {
			// Submit Datadog Event
			_ = c.submitEvent(sender, event)

			// bookmarkSaver manages whether or not to save/persist the bookmark
			err := c.bookmarkSaver.updateBookmark(event)
			if err != nil {
				c.Warnf(err.Error())
			}

			// Must close event handle when we are done with it
			evtapi.EvtCloseRecord(c.evtapi, event.EventRecordHandle)
		}
		return false, nil
	}
}

func (c *Check) submitEvent(sender sender.Sender, event *evtapi.EventRecord) error {
	// Base event
	ddevent := agentEvent.Event{
		Priority:       c.eventPriority,
		SourceTypeName: sourceTypeName,
		Tags:           []string{},
	}

	// Render Windows event values into the DD event
	_ = c.renderEventValues(event, &ddevent)

	// If the event has rendered text, check it against the regexp patterns to see if
	// we should send the event or not
	if len(ddevent.Text) > 0 {
		if !c.includeMessage(ddevent.Text) {
			return nil
		}
	}

	// submit
	sender.Event(ddevent)

	return nil
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
		return fmt.Errorf("configuration error: %v", err)
	}

	// process configuration
	c.config, err = unmarshalConfig(data, initConfig)
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}
	err = c.validateConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %v", err)
	}

	// Start the event subscription
	err = c.initSubscription()
	if err != nil {
		return fmt.Errorf("failed to initialize event subscription: %v", err)
	}

	return nil
}

func (c *Check) validateConfig() error {
	var err error
	c.eventPriority, err = agentEvent.GetEventPriorityFromString(*c.config.instance.EventPriority)
	if err != nil {
		return fmt.Errorf("invalid instance config `event_priority`: %v", err)
	}
	if *c.config.instance.LegacyMode {
		return fmt.Errorf("unsupported configuration: legacy_mode: true")
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
			return fmt.Errorf("invalid instance config `included_messages`: %v", err)
		}
	}

	if c.config.instance.ExcludedMessages != nil {
		c.excludedMessages, err = compileRegexPatterns(c.config.instance.ExcludedMessages)
		if err != nil {
			return fmt.Errorf("invalid instance config `excluded_messages`: %v", err)
		}
	}

	return nil
}

// Cancel stops the check and releases resources
func (c *Check) Cancel() {
	if c.sub != nil {
		c.sub.Stop()
	}

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
