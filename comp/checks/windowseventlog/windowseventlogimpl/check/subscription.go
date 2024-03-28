// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"fmt"
	"sync"

	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

func (c *Check) initSubscription() error {
	var err error

	opts := []evtsubscribe.PullSubscriptionOption{}
	if c.evtapi != nil {
		opts = append(opts, evtsubscribe.WithWindowsEventLogAPI(c.evtapi))
	}

	// The check should have already confirmed that these options are set/valid in validateConfig
	// but since Optional.Get returns multiple values we have to create a new name/variable anyway
	// so we might as well check they are set here, too.
	startMode, isSet := c.config.instance.Start.Get()
	if !isSet {
		return fmt.Errorf("start mode is not set")
	}
	bookmarkFrequency, isSet := c.config.instance.BookmarkFrequency.Get()
	if !isSet {
		return fmt.Errorf("bookmark frequency is not set")
	}
	payloadSize, isSet := c.config.instance.PayloadSize.Get()
	if !isSet {
		return fmt.Errorf("payload size is not set")
	}
	channelPath, isSet := c.config.instance.ChannelPath.Get()
	if !isSet {
		// TODO: handle this better when the profile file is created
		if _, isSet := c.config.instance.DDSecurityEvents.Get(); isSet {
			channelPath = "Security"
		} else {
			return fmt.Errorf("channel path is not set")
		}
	}
	query, isSet := c.config.instance.Query.Get()
	if !isSet {
		return fmt.Errorf("query is not set")
	}

	// Check persistent cache for bookmark
	var bookmark evtbookmark.Bookmark
	bookmarkXML, err := persistentcache.Read(c.bookmarkPersistentCacheKey())
	if err != nil {
		// persistentcache.Read() does not return error if key does not exist
		bookmarkXML = ""
		log.Errorf("error reading bookmark from persistent cache %s, will start at %s events: %v", c.bookmarkPersistentCacheKey(), startMode, err)
	}
	if bookmarkXML != "" {
		// load bookmark
		bookmark, err = evtbookmark.New(
			evtbookmark.WithWindowsEventLogAPI(c.evtapi),
			evtbookmark.FromXML(bookmarkXML))
		if err != nil {
			log.Errorf("error loading bookmark, will start at %s events: %v", startMode, err)
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
		if startMode == "oldest" {
			opts = append(opts, evtsubscribe.WithStartAtOldestRecord())
		}
	}

	// Batch count
	opts = append(opts, evtsubscribe.WithEventBatchCount(uint(payloadSize)))

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
		channelPath,
		query,
		opts...)

	c.bookmarkSaver = &bookmarkSaver{
		sub:               c.sub,
		bookmark:          bookmark,
		bookmarkFrequency: bookmarkFrequency,
		save: func(bookmarkXML string) error {
			err := persistentcache.Write(c.bookmarkPersistentCacheKey(), bookmarkXML)
			if err != nil {
				return fmt.Errorf("failed to persist bookmark: %w", err)
			}
			return nil
		},
	}

	// Create a render context for System event values
	c.systemRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return fmt.Errorf("failed to create system render context: %w", err)
	}

	// Create e render context for UserData/EventData event values
	// render UserData if available, otherise EventData properties are rendered.
	c.userRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextUser)
	if err != nil {
		return fmt.Errorf("failed to create user render context: %w", err)
	}

	return nil
}

func (c *Check) startSubscription() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	err = c.sub.Start()
	if err != nil {
		return fmt.Errorf("failed to start event subscription: %w", err)
	}

	// Start collection loop in the background so we can collect/report
	// events as they come instead of being behind by check_interval (15s).
	c.fetchEventsLoopStop = make(chan struct{})
	c.fetchEventsLoopWaiter.Add(1)
	pipelineWaiter := sync.WaitGroup{}
	eventCh := make(chan *evtapi.EventRecord)
	go c.fetchEventsLoop(eventCh, &pipelineWaiter)

	// start the events or the logs pipeline to handle the incoming events
	if _, isSet := c.config.instance.ChannelPath.Get(); isSet {
		// send events
		c.eventSubmitterPipeline(sender, eventCh, &pipelineWaiter)
	} else if _, isSet := c.config.instance.DDSecurityEvents.Get(); isSet {
		logsAgent, isSet := c.logsAgent.Get()
		if !isSet {
			// validateConfig should prevent this from happening
			return fmt.Errorf("no logs agent available")
		}
		// send logs
		c.logsSubmitterPipeline(logsAgent, eventCh, &pipelineWaiter)
	} else {
		// validateConfig should prevent this from happening
		return fmt.Errorf("neither channel path nor dd_security_events is set")
	}
	return nil
}

func (c *Check) eventSubmitterPipeline(sender sender.Sender, inCh <-chan *evtapi.EventRecord, wg *sync.WaitGroup) {
	eventWithDataCh := c.eventDataGetter(c.fetchEventsLoopStop, inCh, wg)
	eventWithMessageCh := c.eventMessageFilter(c.fetchEventsLoopStop, eventWithDataCh, wg)
	c.ddEventSubmitter(sender, eventWithMessageCh, wg)
}

func (c *Check) logsSubmitterPipeline(logsAgent logsAgent.Component, inCh <-chan *evtapi.EventRecord, wg *sync.WaitGroup) {
	eventWithDataCh := c.eventDataGetter(c.fetchEventsLoopStop, inCh, wg)
	eventDataFilterCh := c.eventDataFilter(c.fetchEventsLoopStop, eventWithDataCh, wg)
	eventWithMessageCh := c.eventMessageFilter(c.fetchEventsLoopStop, eventDataFilterCh, wg)
	c.ddLogSubmitter(logsAgent, c.fetchEventsLoopStop, eventWithMessageCh, wg)
}

func (c *Check) eventDataGetter(doneCh <-chan struct{}, inCh <-chan *evtapi.EventRecord, wg *sync.WaitGroup) <-chan *eventWithData {
	outCh := make(chan *eventWithData)
	eventDataGetter := &eventDataGetter{
		doneCh: doneCh,
		inCh:   inCh,
		outCh:  outCh,
		// eventlog
		evtapi:              c.evtapi,
		systemRenderContext: c.systemRenderContext,
	}
	wg.Add(1)
	go eventDataGetter.run(wg)
	return outCh
}

func (c *Check) eventDataFilter(doneCh <-chan struct{}, inCh <-chan *eventWithData, wg *sync.WaitGroup) <-chan *eventWithData {
	outCh := make(chan *eventWithData)
	eventDataFilter := &eventDataFilter{
		doneCh: doneCh,
		inCh:   inCh,
		outCh:  outCh,
		// TODO: config
		eventIDs: []uint16{4672},
	}
	wg.Add(1)
	go eventDataFilter.run(wg)
	return outCh
}

func (c *Check) eventMessageFilter(doneCh <-chan struct{}, inCh <-chan *eventWithData, wg *sync.WaitGroup) <-chan *eventWithMessage {
	outCh := make(chan *eventWithMessage)
	eventMessageFilter := &eventMessageFilter{
		doneCh: doneCh,
		inCh:   inCh,
		outCh:  outCh,
		// config
		interpretMessages: isaffirmative(c.config.instance.InterpretMessages),
		includedMessages:  c.includedMessages,
		excludedMessages:  c.excludedMessages,
		// eventlog
		userRenderContext: c.userRenderContext,
	}
	wg.Add(1)
	go eventMessageFilter.run(wg)
	return outCh
}

func (c *Check) ddEventSubmitter(sender sender.Sender, inCh <-chan *eventWithMessage, wg *sync.WaitGroup) {
	channelPath := ""
	if val, isSet := c.config.instance.ChannelPath.Get(); isSet {
		channelPath = val
	}
	ddEventSubmitter := &ddEventSubmitter{
		sender:        sender,
		inCh:          inCh,
		bookmarkSaver: c.bookmarkSaver,
		// config
		eventPriority: c.eventPriority,
		remoteSession: c.session != nil,
		channelPath:   channelPath,
		tagEventID:    isaffirmative(c.config.instance.TagEventID),
		tagSID:        isaffirmative(c.config.instance.TagSID),
		// eventlog
		evtapi: c.evtapi,
	}
	wg.Add(1)
	go ddEventSubmitter.run(wg)
}

func (c *Check) ddLogSubmitter(logsAgent logsAgent.Component, doneCh <-chan struct{}, inCh <-chan *eventWithMessage, wg *sync.WaitGroup) {
	ddEventSubmitter := &ddLogSubmitter{
		logsAgent:     logsAgent,
		doneCh:        doneCh,
		inCh:          inCh,
		bookmarkSaver: c.bookmarkSaver,
		logSource: sources.NewLogSource("dd_security_events", &logsConfig.LogsConfig{
			Source:  logsSource,
			Service: "Windows",
		}),
	}
	wg.Add(1)
	go ddEventSubmitter.run(wg)
}

func (c *Check) stopSubscription() {
	if c.sub != nil && c.sub.Running() {
		close(c.fetchEventsLoopStop)
		c.fetchEventsLoopWaiter.Wait()
		// fetchEventsLoop runs c.sub.Stop() when it returns
	}
}

func (c *Check) initSession() error {
	// local session
	if serverIsLocal(c.config.instance.Server) {
		c.session = nil
		return nil
	}

	// remote session
	flags, err := evtRPCFlagsFromOption(c.config.instance.AuthType)
	if err != nil {
		return err
	}

	var server, user, domain, password string
	if val, isSet := c.config.instance.Server.Get(); isSet {
		server = val
	}
	if val, isSet := c.config.instance.User.Get(); isSet {
		user = val
	}
	if val, isSet := c.config.instance.Domain.Get(); isSet {
		domain = val
	}
	if val, isSet := c.config.instance.Password.Get(); isSet {
		password = val
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
