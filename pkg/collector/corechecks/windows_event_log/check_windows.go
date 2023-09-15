// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows
// +build windows

package evtlog

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"

	"golang.org/x/sys/windows"
)

const checkName = "windows_event_log"

// The lower cased version of the `API SOURCE ATTRIBUTE` column from the table located here:
// https://docs.datadoghq.com/integrations/faq/list-of-api-source-attribute-value/
const sourceTypeName = "event viewer"

type Check struct {
	// check
	core.CheckBase
	config *Config

	included_messages []*regexp.Regexp
	excluded_messages []*regexp.Regexp

	// event metrics
	event_priority agentEvent.EventPriority

	// event log
	sub                 evtsubscribe.PullSubscription
	evtapi              evtapi.API
	systemRenderContext evtapi.EventRenderContextHandle
	bookmark            evtbookmark.Bookmark
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

	err = c.fetchEvents(sender)
	if err != nil {
		// An error occurred fetching events, stop the subscription
		c.sub.Stop()
		return fmt.Errorf("failed to fetch events: %v", err)
	}

	sender.Commit()
	return nil
}

func (c *Check) fetchEvents(sender sender.Sender) error {
	var lastEvent *evtapi.EventRecord
	eventsSinceLastBookmark := 0

	// Update the bookmark at the end of the check, regardless of the bookmark_frequency
	defer func() {
		if lastEvent != nil {
			if eventsSinceLastBookmark > 0 {
				err := c.updateBookmark(lastEvent)
				if err != nil {
					c.Warnf("failed to save bookmark: %v", err)
				}
			}
			evtapi.EvtCloseRecord(c.evtapi, lastEvent.EventRecordHandle)
		}
	}()

	// Fetch new events
	for {
		events, err := c.sub.GetEvents()
		if err != nil {
			return err
		}
		if events == nil {
			// no more events
			break
		}
		for i, event := range events {
			// Submit Datadog Event
			_ = c.submitEvent(sender, event)

			// Update bookmark according to bookmark_frequency config
			eventsSinceLastBookmark += 1
			if *c.config.instance.Bookmark_frequency > 0 && eventsSinceLastBookmark >= *c.config.instance.Bookmark_frequency {
				err = c.updateBookmark(event)
				if err != nil {
					c.Warnf("failed to save bookmark: %v", err)
				}
				eventsSinceLastBookmark = 0
			}

			// Close the event handle when we are done with it.
			// If this is the last event in the batch, we may need to use it to update
			// the bookmark so save it until the check finishes.
			if i == len(events)-1 {
				if lastEvent != nil {
					evtapi.EvtCloseRecord(c.evtapi, lastEvent.EventRecordHandle)
				}
				lastEvent = event
			} else {
				evtapi.EvtCloseRecord(c.evtapi, event.EventRecordHandle)
			}
		}
	}

	return nil
}

func (c *Check) submitEvent(sender sender.Sender, event *evtapi.EventRecord) error {
	// Base event
	ddevent := agentEvent.Event{
		Priority:       c.event_priority,
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

// update the bookmark handle to point to event, add the bookmark to the subscription, and then update the persistent cache
func (c *Check) updateBookmark(event *evtapi.EventRecord) error {
	err := c.bookmark.Update(event.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to update bookmark: %v", err)
	}

	c.sub.SetBookmark(c.bookmark)

	bookmarkXML, err := c.bookmark.Render()
	if err != nil {
		return fmt.Errorf("failed to render bookmark XML: %v", err)
	}

	err = persistentcache.Write(c.bookmarkPersistentCacheKey(), bookmarkXML)
	if err != nil {
		return fmt.Errorf("failed to persist bookmark: %v", err)
	}

	return nil
}

func alertTypeFromLevel(level uint64) (agentEvent.EventAlertType, error) {
	// https://docs.microsoft.com/en-us/windows/win32/wes/eventmanifestschema-leveltype-complextype#remarks
	// https://learn.microsoft.com/en-us/windows/win32/wes/eventmanifestschema-eventdefinitiontype-complextype#attributes
	// > If you do not specify a level, the event descriptor will contain a zero for level.
	var alertType string
	switch level {
	case 0:
		alertType = "info"
	case 1:
		alertType = "error"
	case 2:
		alertType = "error"
	case 3:
		alertType = "warning"
	case 4:
		alertType = "info"
	case 5:
		alertType = "info"
	default:
		return agentEvent.EventAlertTypeInfo, fmt.Errorf("invalid event level: '%d'", level)
	}

	return agentEvent.GetAlertTypeFromString(alertType)
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
	if err != nil {
		// default to DD hostname
		fqdn, _ = hostname.Get(context.TODO())
		// TODO: What to do on error?
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
	err = c.renderEventMessage(providerName, winevent, ddevent)
	if err != nil {
		// TODO: continue?
		return err
	}

	// Optional: Tag EventID
	if *c.config.instance.Tag_event_id {
		eventid, err := vals.UInt(evtapi.EvtSystemEventID)
		if err == nil {
			tag := fmt.Sprintf("event_id:%d", eventid)
			ddevent.Tags = append(ddevent.Tags, tag)
		}
	}

	// Optional: Tag SID
	if *c.config.instance.Tag_sid {
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

func (c *Check) renderEventMessage(providerName string, winevent *evtapi.EventRecord, ddevent *agentEvent.Event) error {
	pm, err := c.evtapi.EvtOpenPublisherMetadata(providerName, "")
	if err != nil {
		return err
	}
	defer evtapi.EvtClosePublisherMetadata(c.evtapi, pm)

	message, err := c.evtapi.EvtFormatMessage(pm, winevent.EventRecordHandle, 0, nil, evtapi.EvtFormatMessageEvent)
	if err != nil {
		return err
	}

	ddevent.Text = message

	return nil
}

func (c *Check) includeMessage(message string) bool {
	if len(c.excluded_messages) > 0 {
		for _, re := range c.excluded_messages {
			if re.MatchString(message) {
				// exclude takes precedence over include, so we can stop early
				return false
			}
		}
	}

	if len(c.included_messages) > 0 {
		// include patterns given, message must match a pattern to be included
		for _, re := range c.included_messages {
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
	if *c.config.instance.Bookmark_frequency > 0 {
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
			return err
		}
		opts = append(opts, evtsubscribe.WithStartAfterBookmark(bookmark))
	} else {
		// new bookmark
		bookmark, err = evtbookmark.New(evtbookmark.WithWindowsEventLogAPI(c.evtapi))
		if err != nil {
			return err
		}
		if *c.config.instance.Start == "oldest" {
			opts = append(opts, evtsubscribe.WithStartAtOldestRecord())
		}
	}
	c.bookmark = bookmark

	// Batch count
	opts = append(opts, evtsubscribe.WithEventBatchCount(uint(*c.config.instance.Payload_size)))

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

	// Create a render context for System event values
	c.systemRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return fmt.Errorf("failed to create system render context: %v", err)
	}

	return nil
}

func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	// common CoreCheck requirements
	// This check supports multiple instances, BuildID must be called before CommonConfigure
	c.BuildID(integrationConfigDigest, data, initConfig)
	err := c.CommonConfigure(senderManager, integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return fmt.Errorf("configuraiton error: %v", err)
	}

	// process configuration
	c.config, err = UnmarshalConfig(data, initConfig)
	if err != nil {
		return fmt.Errorf("configuraiton error: %v", err)
	}
	err = c.validateConfig()
	if err != nil {
		return fmt.Errorf("configuraiton error: %v", err)
	}

	// Start the event subscription
	err = c.initSubscription()
	if err != nil {
		return fmt.Errorf("failed to initialize event subscription: %v", err)
	}

	return nil
}

func compileRegexPatterns(patterns []string) ([]*regexp.Regexp, error) {
	var err error
	res := make([]*regexp.Regexp, len(patterns))
	for i, pattern := range patterns {
		res[i], err = regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("error compiling regex pattern '%s': %v", pattern, err)
		}
	}
	return res, nil
}

func (c *Check) validateConfig() error {
	var err error
	c.event_priority, err = agentEvent.GetEventPriorityFromString(*c.config.instance.Event_priority)
	if err != nil {
		return fmt.Errorf("invalid instance config `event_priority`: %v", err)
	}
	if *c.config.instance.Legacy_mode {
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

	if c.config.instance.Included_messages != nil {
		c.included_messages, err = compileRegexPatterns(c.config.instance.Included_messages)
		if err != nil {
			return fmt.Errorf("invalid instance config `included_messages`: %v", err)
		}
	}

	if c.config.instance.Excluded_messages != nil {
		c.excluded_messages, err = compileRegexPatterns(c.config.instance.Excluded_messages)
		if err != nil {
			return fmt.Errorf("invalid instance config `excluded_messages`: %v", err)
		}
	}

	return nil
}

func (c *Check) Cancel() {
	if c.sub != nil {
		c.sub.Stop()
	}

	if c.bookmark != nil {
		c.bookmark.Close()
	}

	if c.systemRenderContext != evtapi.EventRenderContextHandle(0) {
		c.evtapi.EvtClose(windows.Handle(c.systemRenderContext))
	}
}

func checkFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(checkName),
		evtapi:    winevtapi.New(),
	}
}

func init() {
	core.RegisterCheck(checkName, checkFactory)
}
