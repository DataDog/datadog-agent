// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package evtlog defines a check that reads the Windows Event Log and submits Events
package evtlog

import (
	"fmt"

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
