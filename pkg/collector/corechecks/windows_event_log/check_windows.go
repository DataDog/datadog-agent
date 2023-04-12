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
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api/windows"
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
	config Config

	// event metrics
	event_priority metrics.EventPriority

	// event log
	sub                 evtsubscribe.PullSubscription
	evtapi              evtapi.API
	systemRenderContext evtapi.EventRenderContextHandle
}

type Config struct {
	instance instanceConfig
	init     initConfig
}

type instanceConfig struct {
	ChannelPath        string `yaml:"path"`
	Query              string `yaml:"query"`
	Start              string `yaml:"start"`
	Timeout            uint   `yaml:"timeout"`
	Payload_size       uint   `yaml:"payload_size"`
	Bookmark_frequency int    `yaml:"bookmark_frequency"`
	Legacy_mode        bool   `yaml:"legacy_mode"`
	Event_priority     string `yaml:"event_priority"`
	Tag_event_id       bool   `yaml:"tag_event_id"`
	Tag_sid            bool   `yaml:"tag_sid"`
}

type initConfig struct {
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

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
		for _, winevent := range events {
			// Base event
			ddevent := metrics.Event{
				Priority:       c.event_priority,
				SourceTypeName: sourceTypeName,
				Tags:           []string{},
			}

			// Render Windows event values into the DD event
			_ = c.renderEventValues(winevent, &ddevent)

			// submit
			sender.Event(ddevent)

			// cleanup
			evtapi.EvtCloseRecord(c.evtapi, winevent.EventRecordHandle)
		}
	}

	sender.Commit()
	return nil
}

func alertTypeFromLevel(level uint64) (metrics.EventAlertType, error) {
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
		return metrics.EventAlertTypeInfo, fmt.Errorf("Invalid event level: '%d'", level)
	}

	return metrics.GetAlertTypeFromString(alertType)
}

func (c *Check) renderEventValues(winevent *evtapi.EventRecord, ddevent *metrics.Event) error {
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
			alertType, err = metrics.GetAlertTypeFromString("error")
		}
		if err == nil {
			ddevent.AlertType = alertType
		}
	}

	// Provider
	providerName, err := vals.String(evtapi.EvtSystemProviderName)
	if err == nil {
		ddevent.AggregationKey = providerName
		ddevent.Title = fmt.Sprintf("%s/%s", c.config.instance.ChannelPath, providerName)
	}

	// formatted message
	err = c.renderEventMessage(providerName, winevent, ddevent)
	if err != nil {
		// TODO: continue?
		return err
	}

	// Optional: Tag EventID
	if c.config.instance.Tag_event_id {
		eventid, err := vals.UInt(evtapi.EvtSystemEventID)
		if err == nil {
			tag := fmt.Sprintf("event_id:%d", eventid)
			ddevent.Tags = append(ddevent.Tags, tag)
		}
	}

	// Optional: Tag SID
	if c.config.instance.Tag_sid {
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

func (c *Check) renderEventMessage(providerName string, winevent *evtapi.EventRecord, ddevent *metrics.Event) error {
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

func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	// Default values
	c.config.instance.Timeout = 5
	c.config.instance.Legacy_mode = false
	c.config.instance.Payload_size = 10
	c.config.instance.Bookmark_frequency = 10
	c.config.instance.Query = "*"
	c.config.instance.Start = "now"
	c.config.instance.Event_priority = "normal"
	c.config.instance.Tag_event_id = false
	c.config.instance.Tag_sid = false

	// Parse config
	err = yaml.Unmarshal(data, &c.config.instance)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(initConfig, &c.config.init)
	if err != nil {
		return err
	}

	// Validate config
	if len(c.config.instance.ChannelPath) == 0 {
		return fmt.Errorf("instance config `path` must not be empty")
	}
	if c.config.instance.Start != "now" && c.config.instance.Start != "old" {
		return fmt.Errorf("invalid instance config `start`: '%s'", c.config.instance.Start)
	}

	// Default values
	if len(c.config.instance.Query) == 0 {
		c.config.instance.Query = "*"
	}

	// map config options to check options
	c.event_priority, err = metrics.GetEventPriorityFromString(c.config.instance.Event_priority)
	if err != nil {
		return fmt.Errorf("invalid instance config `event_priority`: %v", err)
	}

	// Create the subscription
	opts := []evtsubscribe.PullSubscriptionOption{}
	if c.evtapi != nil {
		opts = append(opts, evtsubscribe.WithWindowsEventLogAPI(c.evtapi))
	}
	if c.config.instance.Start == "old" {
		opts = append(opts, evtsubscribe.WithStartAtOldestRecord())
	}

	opts = append(opts, evtsubscribe.WithEventBatchCount(c.config.instance.Payload_size))

	c.sub = evtsubscribe.NewPullSubscription(
		c.config.instance.ChannelPath,
		c.config.instance.Query,
		opts...)

	// Start the subscription
	err = c.sub.Start()
	if err != nil {
		return fmt.Errorf("Failed to subscribe to events: %v", err)
	}

	// Create a render context for System event values
	c.systemRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return err
	}

	return nil
}

func (c *Check) Cancel() {
	if c.sub != nil {
		c.sub.Stop()
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
