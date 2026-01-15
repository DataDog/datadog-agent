// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"sync"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	publishermetadatacache "github.com/DataDog/datadog-agent/comp/publishermetadatacache/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/util/windowsevent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"

	"golang.org/x/sys/windows"
)

// logsSource is attached to logs submitted by this check and used by Logs pipeline
// to identify Windows Events Logs.
const logsSource = "windows.events"

// ddLogSubmitter transforms Windows Events into strucuted Logs and submits them to the Logs pipeline
type ddLogSubmitter struct {
	doneCh                 <-chan struct{}
	inCh                   <-chan *eventWithMessage
	logsAgent              logsAgent.Component
	bookmarkManager        evtbookmark.Manager
	logSource              *sources.LogSource
	publisherMetadataCache publishermetadatacache.Component
}

func (s *ddLogSubmitter) run(w *sync.WaitGroup) {
	defer w.Done()
	for e := range s.inCh {
		msg, err := s.getLogMessage(e)
		if err != nil {
			log.Errorf("%v", err)
			e.Close()
			continue
		}

		select {
		case s.logsAgent.GetPipelineProvider().NextPipelineChan() <- msg:
		case <-s.doneCh:
			e.Close()
			return
		}

		// bookmarkManager manages whether or not to save/persist the bookmark
		err = s.bookmarkManager.UpdateAndSave(e.winevent.EventRecordHandle)
		if err != nil {
			log.Errorf("%v", err)
		}

		// Must close event handle when we are done with it
		e.Close()
	}
}

func (s *ddLogSubmitter) getLogMessage(e *eventWithMessage) (*message.Message, error) {
	xmlData, err := e.evtapi.EvtRenderEventXml(e.winevent.EventRecordHandle)
	if err != nil {
		return nil, fmt.Errorf("Error rendering xml: %v", err)
	}
	xml := windows.UTF16ToString(xmlData)

	m, err := windowsevent.NewMapXML([]byte(xml))
	if err != nil {
		return nil, fmt.Errorf("Error creating map from xml: %v", err)
	}

	err = s.enrichEvent(m, e)
	if err != nil {
		log.Errorf("%v", err)
		// continue to submit the event even if we failed to enrich it
	}

	msg, err := windowsevent.MapToMessage(m, s.logSource, true)
	if err != nil {
		return nil, fmt.Errorf("Failed to convert xml to json: %v for event %s", err, xml)
	}

	return msg, nil
}

func (s *ddLogSubmitter) enrichEvent(m *windowsevent.Map, e *eventWithMessage) error {
	providerName, err := e.systemVals.String(evtapi.EvtSystemProviderName)
	if err != nil {
		return fmt.Errorf("Failed to get provider name: %v", err)
	}

	eh := e.winevent.EventRecordHandle
	task, _ := s.publisherMetadataCache.FormatMessage(providerName, eh, evtapi.EvtFormatMessageTask)
	opcode, _ := s.publisherMetadataCache.FormatMessage(providerName, eh, evtapi.EvtFormatMessageOpcode)
	level, _ := s.publisherMetadataCache.FormatMessage(providerName, eh, evtapi.EvtFormatMessageLevel)

	_ = m.SetMessage(e.renderedMessage)
	_ = m.SetTask(task)
	_ = m.SetOpcode(opcode)
	_ = m.SetLevel(level)

	return nil
}
