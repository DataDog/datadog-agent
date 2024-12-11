// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	agentEvent "github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// The lower cased version of the `API SOURCE ATTRIBUTE` column from the table located here:
// https://docs.datadoghq.com/integrations/faq/list-of-api-source-attribute-value/
const sourceTypeName = "event viewer"

// ddEventSubmitter transforms Windows events into Datadog events and submits them to the sender
type ddEventSubmitter struct {
	sender        sender.Sender
	inCh          <-chan *eventWithMessage
	bookmarkSaver *bookmarkSaver

	// config
	eventPriority agentEvent.Priority
	remoteSession bool
	channelPath   string
	tagEventID    bool
	tagSID        bool

	// eventlog
	evtapi evtapi.API
}

func (s *ddEventSubmitter) run(w *sync.WaitGroup) {
	defer w.Done()
	for e := range s.inCh {
		s.submit(e)

		// bookmarkSaver manages whether or not to save/persist the bookmark
		err := s.bookmarkSaver.updateBookmark(e.winevent)
		if err != nil {
			log.Warnf("%v", err)
		}

		// Must close event handle when we are done with it
		e.Close()
	}
}

func (s *ddEventSubmitter) submit(e *eventWithMessage) {
	// Base event
	ddevent := agentEvent.Event{
		Priority:       s.eventPriority,
		SourceTypeName: sourceTypeName,
		Tags:           []string{},
	}

	// Render Windows event values into the DD event
	err := s.renderEventValues(e.systemVals, e.renderedMessage, &ddevent)
	if err != nil {
		log.Error(err)
	}

	// submit
	s.sender.Event(ddevent)
}

func (s *ddEventSubmitter) renderEventValues(vals evtapi.EvtVariantValues, renderedMessage string, ddevent *agentEvent.Event) error {
	// Timestamp
	ts, err := vals.Time(evtapi.EvtSystemTimeCreated)
	if err != nil {
		// if no timestamp default to current time
		ts = time.Now().UTC().Unix()
	}
	ddevent.Ts = ts
	// FQDN
	fqdn, err := vals.String(evtapi.EvtSystemComputer)
	if err != nil || !s.remoteSession {
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
		if len(s.channelPath) > 0 {
			ddevent.Title = fmt.Sprintf("%s/%s", s.channelPath, providerName)
		}
	}

	// formatted message
	if len(renderedMessage) > 0 {
		ddevent.Text = renderedMessage
	}

	// Optional: Tag EventID
	if s.tagEventID {
		eventid, err := vals.UInt(evtapi.EvtSystemEventID)
		if err == nil {
			tag := fmt.Sprintf("event_id:%d", eventid)
			ddevent.Tags = append(ddevent.Tags, tag)
		}
	}

	// Optional: Tag SID
	if s.tagSID {
		sid, err := vals.SID(evtapi.EvtSystemUserID)
		if err == nil {
			// https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-lookupaccountsidw
			// TODO WINA-513: LookupAccountName takes 30 seconds to timeout, which will significantly
			//       slow down event collection if it consistently fails.
			var account, domain string
			if !s.remoteSession {
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

func alertTypeFromLevel(level uint64) (agentEvent.AlertType, error) {
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
		return agentEvent.AlertTypeInfo, fmt.Errorf("invalid event level: '%d'", level)
	}

	return agentEvent.GetAlertTypeFromString(alertType)
}
