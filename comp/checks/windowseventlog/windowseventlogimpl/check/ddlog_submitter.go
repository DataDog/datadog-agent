// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"sync"
	"time"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
)

// TODO:
const logsSource = "windows.events"

// ddLogSubmitter transforms Windows Events into strucuted Logs and submits them to the Logs pipeline
type ddLogSubmitter struct {
	logsAgent     logsAgent.Component
	inCh          <-chan *eventWithMessage
	bookmarkSaver *bookmarkSaver

	// eventlog
	evtapi evtapi.API
}

func (s *ddLogSubmitter) run(w *sync.WaitGroup) {
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

func (s *ddLogSubmitter) submit(e *eventWithMessage) {
	m := message.NewMessageWithSource(
		[]byte(e.renderedMessage),
		"status",
		// TODO: this might need to be persistent
		sources.NewLogSource("windows event log", &logsConfig.LogsConfig{
			Source:  logsSource,
			Service: logsSource,
		}),
		time.Now().UnixNano(),
	)
	s.logsAgent.GetPipelineProvider().NextPipelineChan() <- m
}
