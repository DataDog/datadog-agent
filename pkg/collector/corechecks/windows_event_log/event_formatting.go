// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"strings"
	"time"

	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

func (c *Check) renderEventValues(winevent *evtapi.EventRecord, ddevent *agentEvent.Event) error {
	// Render the values
	vals, err := c.evtapi.EvtRenderEventValues(c.systemRenderContext, winevent.EventRecordHandle)
	if err != nil {
		return fmt.Errorf("failed to render values: %w", err)
	}
	defer vals.Close()

	// Timestamp
	ts, err := vals.Time(evtapi.EvtSystemTimeCreated)
	if err != nil {
		// if no timestamp default to current time
		ts = time.Now().UTC().Unix()
	}
	ddevent.Ts = ts
	// FQDN
	fqdn, err := vals.String(evtapi.EvtSystemComputer)
	if err != nil || c.session == nil {
		// use default hostname provided by aggregator.Sender
		//   * if collecting from local computer
		//   * if fail to fetch hostname of remote computer
		fqdn = ""
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
		if val, isSet := c.config.instance.ChannelPath.Get(); isSet {
			ddevent.Title = fmt.Sprintf("%s/%s", val, providerName)
		}
	}

	// formatted message
	message, err := c.getEventMessage(providerName, winevent)
	if err != nil {
		log.Errorf("failed to get event message: %v", err)
	} else {
		ddevent.Text = message
	}

	// Optional: Tag EventID
	if isaffirmative(c.config.instance.TagEventID) {
		eventid, err := vals.UInt(evtapi.EvtSystemEventID)
		if err == nil {
			tag := fmt.Sprintf("event_id:%d", eventid)
			ddevent.Tags = append(ddevent.Tags, tag)
		}
	}

	// Optional: Tag SID
	if isaffirmative(c.config.instance.TagSID) {
		sid, err := vals.SID(evtapi.EvtSystemUserID)
		if err == nil {
			// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-lookupaccountsidw
			// TODO WINA-513: LookupAccountName takes 30 seconds to timeout, which will significantly
			//       slow down event collection if it consistently fails.
			var account, domain string
			if c.session == nil {
				// "local" lookup, may contact trusted domain controllers
				account, domain, _, err = sid.LookupAccount("")
			} else {
				// remote lookup, requires LSARPC via SMB/RPC
				var host string
				host, err = vals.String(evtapi.EvtSystemComputer)
				if err == nil {
					account, domain, _, err = sid.LookupAccount(host)
				} else {
					err = fmt.Errorf("failed to get host from event: %w", err)
				}
			}
			if err == nil {
				tag := fmt.Sprintf("sid:%s\\%s", domain, account)
				ddevent.Tags = append(ddevent.Tags, tag)
			} else {
				log.Errorf("failed to lookup user for sid '%s': %v", sid.String(), err)
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
	if isaffirmative(c.config.instance.InterpretMessages) {
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

	// Remove invisible unicode character from message
	// https://unicode.scarfboy.com/?s=U%2b200E
	// https://github.com/mhammond/pywin32/pull/1524#issuecomment-633152961
	message = strings.ReplaceAll(message, "\u200e", "")

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
