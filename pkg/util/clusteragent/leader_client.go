// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"net/http"
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
	panic("not called")
}

// getBaseURL returns the url to query: the last known url
// of the leader, or the main url if not known
func (l *leaderClient) getBaseURL() string {
	panic("not called")
}

// hasLeader returns true if a leader is in cache,
// false if requests will go through the service
func (l *leaderClient) hasLeader() bool {
	panic("not called")
}

// buildURL is a convenience method to create a full url by
// adding path parts to the base url
func (l *leaderClient) buildURL(parts ...string) string {
	panic("not called")
}

// resetURL has to be called on errors to fallback
// to the serviceURL when the leader churns away.
func (l *leaderClient) resetURL() {
	panic("not called")
}

// redirected is passed to the http client to cache leader
// redirections for future queries.
func (l *leaderClient) redirected(req *http.Request, via []*http.Request) error {
	panic("not called")
}
