// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

package evtlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/checks/windowseventlog/impl/check/eventdatafilter"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	logsConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	evtapi "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/api"
	evtbookmark "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/bookmark"
	evtsession "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/session"
	evtsubscribe "github.com/DataDog/datadog-agent/pkg/util/winutil/eventlog/subscription"
)

func (c *Check) getChannelPath() (string, error) {
	if c.ddSecurityEventsFilter != nil {
		return "Security", nil
	}
	if val, isSet := c.config.instance.ChannelPath.Get(); isSet {
		return val, nil
	}
	return "", errors.New("channel path is not set")
}

func (c *Check) initSubscription() error {
	var err error

	// The check should have already confirmed that these options are set/valid in validateConfig
	// but since Optional.Get returns multiple values we have to create a new name/variable anyway
	// so we might as well check they are set here, too.
	startMode, isSet := c.config.instance.Start.Get()
	if !isSet {
		return errors.New("start mode is not set")
	}
	bookmarkFrequency, isSet := c.config.instance.BookmarkFrequency.Get()
	if !isSet {
		return errors.New("bookmark frequency is not set")
	}
	payloadSize, isSet := c.config.instance.PayloadSize.Get()
	if !isSet {
		return errors.New("payload size is not set")
	}
	channelPath, err := c.getChannelPath()
	if err != nil {
		return err
	}
	query, isSet := c.config.instance.Query.Get()
	if !isSet {
		return errors.New("query is not set")
	}

	// Create BookmarkManager for handling bookmark lifecycle
	// The manager will handle loading existing bookmarks and creating new ones as needed
	// Note: Bookmark initialization will happen in startSubscription() on first Run()
	bookmarkSaver := newPersistentCacheSaver(c.bookmarkPersistentCacheKey())
	c.bookmarkManager = evtbookmark.NewManager(evtbookmark.Config{
		API:               c.evtapi,
		Saver:             bookmarkSaver,
		BookmarkFrequency: bookmarkFrequency,
	})

	// Initialize session
	err = c.initSession()
	if err != nil {
		return err
	}

	// Build subscription options
	opts := []evtsubscribe.PullSubscriptionOption{}
	if c.evtapi != nil {
		opts = append(opts, evtsubscribe.WithWindowsEventLogAPI(c.evtapi))
	}

	// Add bookmark saver - subscription will load/save bookmark on Start()
	opts = append(opts, evtsubscribe.WithBookmarkSaver(bookmarkSaver))

	// Add start mode - subscription will use this if no bookmark is loaded
	opts = append(opts, evtsubscribe.WithStartMode(startMode))

	// Batch count
	opts = append(opts, evtsubscribe.WithEventBatchCount(uint(payloadSize)))

	// Session
	if c.session != nil {
		opts = append(opts, evtsubscribe.WithSession(c.session))
	}

	// Create the subscription (not started yet)
	// Bookmark will be loaded and initialized in subscription.Start()
	c.sub = evtsubscribe.NewPullSubscription(channelPath, query, opts...)

	// Create a render context for System event values
	c.systemRenderContext, err = c.evtapi.EvtCreateRenderContext(nil, evtapi.EvtRenderContextSystem)
	if err != nil {
		return fmt.Errorf("failed to create system render context: %w", err)
	}

	// Create a render context for UserData/EventData event values
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

	// Start the subscription
	// The subscription will handle bookmark loading and initialization internally
	err = c.sub.Start()
	if err != nil {
		return err
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
		// send logs
		err = c.logsSubmitterPipeline(eventCh, &pipelineWaiter)
		if err != nil {
			return err
		}
	} else {
		// validateConfig should prevent this from happening
		return errors.New("neither channel path nor dd_security_events is set")
	}
	return nil
}

func (c *Check) eventSubmitterPipeline(sender sender.Sender, inCh <-chan *evtapi.EventRecord, wg *sync.WaitGroup) {
	eventWithDataCh := c.eventDataGetter(c.fetchEventsLoopStop, inCh, wg)
	// rendering the message is expensive so ensure any filtering that does not require
	// the message is done earlier.
	eventWithMessageCh := c.eventMessageFilter(c.fetchEventsLoopStop, eventWithDataCh, wg)
	c.ddEventSubmitter(sender, eventWithMessageCh, wg)
}

func (c *Check) logsSubmitterPipeline(inCh <-chan *evtapi.EventRecord, wg *sync.WaitGroup) error {
	logsAgent, isSet := c.logsAgent.Get()
	if !isSet {
		// sanity: validateConfig should prevent this from happening
		return errors.New("no logs agent available")
	}

	if c.ddSecurityEventsFilter == nil {
		// sanity: validateConfig should prevent this from happening
		return errors.New("no security profile loaded")
	}

	eventWithDataCh := c.eventDataGetter(c.fetchEventsLoopStop, inCh, wg)
	eventDataFilterCh := c.eventDataFilter(c.ddSecurityEventsFilter, c.fetchEventsLoopStop, eventWithDataCh, wg)
	// rendering the message is expensive so ensure any filtering that does not require
	// the message is done earlier.
	eventWithMessageCh := c.eventMessageFilter(c.fetchEventsLoopStop, eventDataFilterCh, wg)
	c.ddLogSubmitter(logsAgent, c.fetchEventsLoopStop, eventWithMessageCh, wg)
	return nil
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

func (c *Check) eventDataFilter(filter eventdatafilter.Filter, doneCh <-chan struct{}, inCh <-chan *eventWithData, wg *sync.WaitGroup) <-chan *eventWithData {
	outCh := make(chan *eventWithData)
	eventDataFilter := &eventDataFilter{
		doneCh: doneCh,
		inCh:   inCh,
		outCh:  outCh,
		filter: filter,
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
		userRenderContext:      c.userRenderContext,
		publisherMetadataCache: c.publisherMetadataCache,
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
		sender:          sender,
		inCh:            inCh,
		bookmarkManager: c.bookmarkManager,
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
		logsAgent:       logsAgent,
		doneCh:          doneCh,
		inCh:            inCh,
		bookmarkManager: c.bookmarkManager,
		logSource: sources.NewLogSource("dd_security_events", &logsConfig.LogsConfig{
			Source: logsSource,
		}),
		publisherMetadataCache: c.publisherMetadataCache,
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

func (c *Check) getProfilesDir() (string, error) {
	root := c.ConfigSource()
	if strings.HasPrefix(root, "file:") {
		root = strings.TrimPrefix(root, "file:")
		root = filepath.Dir(root)
	} else {
		root = c.agentConfig.GetString("confd_path")
		if root == "" {
			return "", errors.New("confd_path is not set")
		}
		root = filepath.Join(root, CheckName+".d")
	}
	return filepath.Join(root, "profiles"), nil
}

func mapSecurityEventLevelToProfileFile(level string) (string, error) {
	switch level {
	case "low":
		return "dd_security_events_low.yaml", nil
	case "high":
		return "dd_security_events_high.yaml", nil
	}
	return "", fmt.Errorf("invalid security level: %s", level)
}

func (c *Check) loadDDSecurityProfile(level string) (eventdatafilter.Filter, error) {
	// get the path to the security profile
	profilesDir, err := c.getProfilesDir()
	if err != nil {
		return nil, err
	}
	profileName, err := mapSecurityEventLevelToProfileFile(level)
	if err != nil {
		return nil, err
	}
	profilePath := filepath.Join(profilesDir, profileName)
	log.Infof("Loading security profile from %s", profilePath)

	// read the profile
	reader, err := os.Open(profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open profile: %w", err)
	}
	defer reader.Close()
	yamlData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	f, err := eventdatafilter.NewFilterFromConfig(yamlData)
	if err != nil {
		return nil, fmt.Errorf("failed to load security profile: %w", err)
	}
	return f, nil
}
