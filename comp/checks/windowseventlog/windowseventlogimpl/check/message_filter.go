// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// eventWithMessage is an event record with rendered system values and message
type eventWithMessage struct {
	*eventWithData
	renderedMessage string
}

// Close frees resources associated with the event
func (e *eventWithMessage) Close() {
	if e.eventWithData != nil {
		e.eventWithData.Close()
	}
}

// eventMessageFilter filters Windows Events based on the rendered message content.
// Events that are not output are closed.
type eventMessageFilter struct {
	doneCh <-chan struct{}
	inCh   <-chan *eventWithData
	outCh  chan<- *eventWithMessage

	// config
	includedMessages  []*regexp.Regexp
	excludedMessages  []*regexp.Regexp
	interpretMessages bool

	// contexts
	userRenderContext evtapi.EventRenderContextHandle
}

func (f *eventMessageFilter) run(w *sync.WaitGroup) {
	defer w.Done()
	defer close(f.outCh)
	for d := range f.inCh {
		e := &eventWithMessage{
			eventWithData: d,
		}

		err := f.renderEvent(e)
		if err == nil {
			// If the event has rendered text, check it against the regexp patterns to see if
			// we should send the event or not
			if !f.includeMessage(e.renderedMessage) {
				// event did not pass filter, do not output it
				e.Close()
				continue
			}
		} else {
			log.Errorf("failed to render event: %v", err)
			// If we failed to render the event, we can't check the message content
			// but we still want to output the event.
			// This is how the check has historically functioned, we could consider
			// adding an option to control this behavior.
		}

		select {
		case f.outCh <- e:
		case <-f.doneCh:
			e.Close()
			return
		}
	}
}

func (f *eventMessageFilter) renderEvent(e *eventWithMessage) error {
	d := e.eventWithData
	// Provider
	providerName, err := d.systemVals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("failed to get provider name: %w", err)
	}

	e.renderedMessage, err = f.getEventMessage(d.evtapi, providerName, d.winevent)
	if err != nil {
		return fmt.Errorf("failed to get event message: %w", err)
	}

	return nil
}

func (f *eventMessageFilter) getEventMessage(api evtapi.API, providerName string, winevent *evtapi.EventRecord) (string, error) {
	var message string

	// Try to render the message via the event log API
	pm, err := api.EvtOpenPublisherMetadata(providerName, "")
	if err == nil {
		defer evtapi.EvtClosePublisherMetadata(api, pm)

		message, err = api.EvtFormatMessage(pm, winevent.EventRecordHandle, 0, nil, evtapi.EvtFormatMessageEvent)
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
	if f.interpretMessages {
		// Render the values
		var eventValues evtapi.EvtVariantValues
		eventValues, err = api.EvtRenderEventValues(f.userRenderContext, winevent.EventRecordHandle)
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

func (f *eventMessageFilter) includeMessage(message string) bool {
	if len(f.excludedMessages) > 0 {
		for _, re := range f.excludedMessages {
			if re.MatchString(message) {
				// exclude takes precedence over include, so we can stop early
				return false
			}
		}
	}

	if len(f.includedMessages) > 0 {
		// include patterns given, message must match a pattern to be included
		for _, re := range f.includedMessages {
			if re.MatchString(message) {
				return true
			}
		}
		// message did not match any patterns
		return false
	}

	return true
}
