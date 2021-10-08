// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// leaderClient is used to keep track of the leading cluster-agent
// to preferentially direct requests to it.
// Test coverage is ensured by TestClusterChecksRedirect
type leaderClient struct {
	http.Client
	m          sync.Mutex
	serviceURL string // Common URL to fallback to
	leaderURL  string // Current leader URL
}

func newLeaderClient(mainClient *http.Client, serviceURL string) *leaderClient {
	l := &leaderClient{
		Client:     *mainClient,
		serviceURL: serviceURL,
	}
	l.CheckRedirect = l.redirected
	return l
}

// getBaseURL returns the url to query: the last known url
// of the leader, or the main url if not known
func (l *leaderClient) getBaseURL() string {
	l.m.Lock()
	defer l.m.Unlock()

	if l.leaderURL == "" {
		return l.serviceURL
	}
	return l.leaderURL
}

// hasLeader returns true if a leader is in cache,
// false if requests will go through the service
func (l *leaderClient) hasLeader() bool {
	l.m.Lock()
	defer l.m.Unlock()
	return l.leaderURL != ""
}

// buildURL is a convenience method to create a full url by
// adding path parts to the base url
func (l *leaderClient) buildURL(parts ...string) string {
	urlParts := []string{l.getBaseURL()}
	urlParts = append(urlParts, parts...)

	return strings.Join(urlParts, "/")
}

// resetURL has to be called on errors to fallback
// to the serviceURL when the leader churns away.
func (l *leaderClient) resetURL() {
	l.m.Lock()
	defer l.m.Unlock()
	l.leaderURL = ""
}

// redirected is passed to the http client to cache leader
// redirections for future queries.
func (l *leaderClient) redirected(req *http.Request, via []*http.Request) error {
	// Copy of default behaviour to avoid infinite redirects
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}

	// Continue passing the bearer token in headers
	if len(via) == 0 {
		return errors.New("cannot find previous request")
	}
	req.Header = via[0].Header

	// Cache the target host for future requests
	l.m.Lock()
	defer l.m.Unlock()
	newURL := &url.URL{
		Scheme: req.URL.Scheme,
		Host:   req.URL.Host,
	}
	l.leaderURL = newURL.String()

	return nil
}
