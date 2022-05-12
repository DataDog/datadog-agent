// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/tls"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"net/http/httputil"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
)

const (
	forwardHeader = "X-DCA-Follower-Forwarded"
	respForwarded = "X-DCA-Forwarded"
)

// LeaderForwarder allows to forward queries from follower to leader
type LeaderForwarder struct {
	transport *http.Transport
	logger    *stdLog.Logger
	proxy     *httputil.ReverseProxy
	proxyLock sync.RWMutex
	apiPort   string
}

// NewLeaderForwarder returns a new LeaderForwarder
func NewLeaderForwarder(apiPort, maxConnections, maxIdleConnections int) *LeaderForwarder {
	lf := &LeaderForwarder{
		apiPort: strconv.Itoa(apiPort),
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 20 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     false,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			TLSHandshakeTimeout:   2 * time.Second,
			MaxConnsPerHost:       maxConnections,
			MaxIdleConns:          maxIdleConnections,
			IdleConnTimeout:       30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		logger: stdLog.New(&config.ErrorLogWriter{
			AdditionalDepth: 4, // Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
		}, "Error while forwarding to leader DCA: ", 0), // log errors to seelog,
	}

	return lf
}

// Forward forwards a query to leader if available
func (lf *LeaderForwarder) Forward(rw http.ResponseWriter, req *http.Request) {
	// Always set Forwarded header in reply
	rw.Header().Set(respForwarded, "true")

	if req.Header.Get(forwardHeader) != "" {
		http.Error(rw, fmt.Sprintf("Query was already forwarded from: %s", req.RemoteAddr), http.StatusLoopDetected)
	}

	var currentProxy *httputil.ReverseProxy
	lf.proxyLock.RLock()
	currentProxy = lf.proxy
	lf.proxyLock.RUnlock()

	if currentProxy == nil {
		http.Error(rw, "", http.StatusServiceUnavailable)
		return
	}
	currentProxy.ServeHTTP(rw, req)
}

// SetLeaderIP allows to change the target leader IP
func (lf *LeaderForwarder) SetLeaderIP(leaderIP string) {
	lf.proxyLock.Lock()
	defer lf.proxyLock.Unlock()

	if leaderIP == "" {
		lf.proxy = nil
	}

	lf.proxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "https"
			req.URL.Host = leaderIP + ":" + lf.apiPort
			req.Header.Add(forwardHeader, "true")
		},
		Transport: lf.transport,
		ErrorLog:  lf.logger,
	}
}
