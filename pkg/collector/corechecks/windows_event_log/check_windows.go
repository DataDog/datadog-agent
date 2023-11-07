// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	agentCheck "github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
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

	err = c.fetchEventsLoop(sender)
	if err != nil {
		// An error occurred fetching events, stop the subscription
		c.sub.Stop()
		return fmt.Errorf("failed to fetch events: %v", err)
	}

	sender.Commit()
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
		// Waited for timeout for did not receive any new events, end the check
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

func (c *Check) renderEventValues(winevent *evtapi.EventRecord, ddevent *agentEvent.Event) error {
	// Render the values
	vals, err := c.evtapi.EvtRenderEventValues(c.systemRenderContext, winevent.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render values: %v", err)
	}
	defer vals.Close()

	// Timestamp
	ts, err := vals.Time(evtapi.EvtSystemTimeCreated)
	if err != nil {
		// if no timestamp default to current time
		ts = time.Now().Unix()
	}
	ddevent.Ts = ts
	// FQDN
	fqdn, err := vals.String(evtapi.EvtSystemComputer)
	if err != nil || c.session == nil {
		// use DD hostname
		//   * if collecting from local computer
		//   * if fail to fetch hostname of remote computer
		fqdn, _ = hostname.Get(context.TODO())
		// on error/empty, default hostname is provided by aggregator.Sender
	}
	ddevent.Host = fqdn
	// Level
	level, err := vals.UInt(evtapi.EvtSystemLevel)
	if err == nil {
		// python compat: only set AlertType if level exists
		alertType, err := alertTypeFromLevel(level)
		if err != nil {
			// if not a valid level, default to error
			alertType, err = agentEvent.GetAlertTypeFromString("error")
		}
		if err == nil {
			ddevent.AlertType = alertType
		}
	}

	// Provider
	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err == nil {
		ddevent.AggregationKey = providerName
		ddevent.Title = fmt.Sprintf("%s/%s", *c.config.instance.ChannelPath, providerName)
	}

	// formatted message
	message, err := c.getEventMessage(providerName, winevent)
	if err != nil {
		log.Errorf("failed to get event message: %v", err)
	} else {
		ddevent.Text = message
	}

	// Optional: Tag EventID
	if *c.config.instance.TagEventID {
		eventid, err := vals.UInt(evtapi.EvtSystemEventID)
		if err == nil {
			tag := fmt.Sprintf("event_id:%d", eventid)
			ddevent.Tags = append(ddevent.Tags, tag)
		}
	}

	// Optional: Tag SID
	if *c.config.instance.TagSID {
		sid, err := vals.SID(evtapi.EvtSystemUserID)
		if err == nil {
			account, domain, _, err := sid.LookupAccount("")
			if err == nil {
				tag := fmt.Sprintf("sid:%s\\%s", domain, account)
				ddevent.Tags = append(ddevent.Tags, tag)
			}
		}
	}

	return nil
}

func (c *Check) getEventMessage(providerName string, winevent *evtapi.EventRecord) (string, error) {
	var message string

	// Try to render the message via the event log API
	pm, err := c.evtapi.EvtOpenPublisherMetadata(providerName, "")
	if err == nil {
		defer evtapi.EvtClosePublisherMetadata(c.evtapi, pm)

		message, err = c.evtapi.EvtFormatMessage(pm, winevent.EventRecordHandle, 0, nil, evtapi.EvtFormatMessageEvent)
		if err == nil {
			return message, nil
		}
		err = fmt.Errorf("failed to render message: %w", err)
	} else {
		err = fmt.Errorf("failed to open event publisher: %w", err)
	}
	renderErr := err

	// rendering failed, which may happen if
	// * the event source/provider cannot be found/loaded
	// * Code 15027: The message resource is present but the message was not found in the message table.
	// * Code 15028: The message ID for the desired message could not be found.
	// Optional: try to provide some information by including any strings from the EventData in the message.
	if *c.config.instance.InterpretMessages {
		// Render the values
		var eventValues evtapi.EvtVariantValues
		eventValues, err = c.evtapi.EvtRenderEventValues(c.userRenderContext, winevent.EventRecordHandle)
		if err == nil {
			defer eventValues.Close()
			// aggregate the string values
			var msgstrings []string
			for i := uint(0); i < eventValues.Count(); i++ {
				val, err := eventValues.String(i)
				if err == nil && len(val) > 0 {
					msgstrings = append(msgstrings, val)
				}
			}
			if len(msgstrings) > 0 {
				message = strings.Join(msgstrings, "\n")
			} else {
				err = fmt.Errorf("no strings in EventData, and %w", renderErr)
			}
		} else {
			err = fmt.Errorf("failed to render EventData, and %w", renderErr)
		}
	}

	if message == "" {
		return "", err
	}

	return message, nil
}

func (c *Check) includeMessage(message string) bool {
	if len(c.excludedMessages) > 0 {
		for _, re := range c.excludedMessages {
			if re.MatchString(message) {
				// exclude takes precedence over include, so we can stop early
				return false
			}
		}
	}

	if len(c.includedMessages) > 0 {
		// include patterns given, message must match a pattern to be included
		for _, re := range c.includedMessages {
			if re.MatchString(message) {
				return true
			}
		}
		// message did not match any patterns
		return false
	}

	return true
}

func (c *Check) initSubscription() error {
	var err error

	opts := []evtsubscribe.PullSubscriptionOption{}
	if c.evtapi != nil {
		opts = append(opts, evtsubscribe.WithWindowsEventLogAPI(c.evtapi))
	}

	// Check persistent cache for bookmark
	var bookmark evtbookmark.Bookmark
	bookmarkXML := ""
	if *c.config.instance.BookmarkFrequency > 0 {
		bookmarkXML, err = persistentcache.Read(c.bookmarkPersistentCacheKey())
		if err != nil {
			// persistentcache.Read() does not return error if key does not exist
			return fmt.Errorf("error reading bookmark from persistent cache %s: %v", c.bookmarkPersistentCacheKey(), err)
		}
	}
	if bookmarkXML != "" {
		// load bookmark
		bookmark, err = evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(c.evtapi),
			evtbookmark.FromXML(bookmarkXML))
		if err != nil {
			log.Errorf("error loading bookmark, will start at %s events: %v", *c.config.instance.Start, err)
		} else {
			opts = append(opts, evtsubscribe.WithStartAfterBookmark(bookmark))
		}
	}
	if bookmark == nil {
		// new bookmark
		bookmark, err = evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(c.evtapi))
		if err != nil {
			return err
		}
		if *c.config.instance.Start == "oldest" {
			opts = append(opts, evtsubscribe.WithStartAtOldestRecord())
		}
	}

	c.bookmarkSaver = &bookmarkSaver{
		bookmark:          bookmark,
		bookmarkFrequency: *c.config.instance.BookmarkFrequency,
		saveBookmark: func(bookmarkXML string) error {
			err := persistentcache.Write(c.bookmarkPersistentCacheKey(), bookmarkXML)
			if err != nil {
				return fmt.Errorf("failed to persist bookmark: %v", err)
			}
			return nil
		},
	}

	// Batch count
	opts = append(opts, evtsubscribe.WithEventBatchCount(uint(*c.config.instance.PayloadSize)))

	// session
	err = c.initSession()
	if err != nil {
		return err
	}

	if c.session != nil {
		opts = append(opts, evtsubscribe.WithSession(c.session))
	}

	// Create the subscription
	c.sub = evtsubscribe.NewPullSubscription(
		*c.config.instance.ChannelPath,
		*c.config.instance.Query,
		opts...)

	// Start the subscription
	err = c.sub.Start()
	if err != nil {
		return fmt.Errorf("failed to subscribe to events: %v", err)
	}

	// Connect bookmark to subscription now in case we didn't load a bookmark when the
	// subscription was created.
	c.sub.SetBookmark(bookmark)

	// Create a render context for System event values
	c.systemRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return fmt.Errorf("failed to create system render context: %v", err)
	}

	// Create e render context for UserData/EventData event values
	// render UserData if available, otherise EventData properties are rendered.
	c.userRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextUser)
	if err != nil {
		return fmt.Errorf("failed to create user render context: %v", err)
	}

	return nil
}

func (c *Check) initSession() error {
	// local session
	if serverIsLocal(c.config.instance.Server) {
		c.session = nil
		return nil
	}

	// remote session
	flags, err := evtRPCFlagsFromString(*c.config.instance.AuthType)
	if err != nil {
		return err
	}
	var server, user, domain, password string
	if c.config.instance.Server != nil {
		server = *c.config.instance.Server
	}
	if c.config.instance.User != nil {
		user = *c.config.instance.User
	}
	if c.config.instance.Domain != nil {
		domain = *c.config.instance.Domain
	}
	if c.config.instance.Password != nil {
		password = *c.config.instance.Password
	}
	session, err := evtsession.NewRemote(
		c.evtapi,
		server,
		user,
		domain,
		password,
		flags,
	)
	if err != nil {
		return err
	}
	c.session = session
	return nil
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
