// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package api contains the telemetry of the Cluster Agent API and implements
// the forwarding of queries from Cluster Agent followers to the leader.
package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var forwardedRequest = telemetry.NewCounterWithOpts("", "forwarded_requests",
	[]string{"handler"}, "Counter of requests forwarded to the dca leader",
	telemetry.Options{NoDoubleUnderscoreSep: true})

// RequestPreHandler is a function that validates a http request and returns true if the request should be handled
// otherwise it returns false and answers to the request.
type RequestPreHandler func(w http.ResponseWriter, r *http.Request) bool

// RequestHandler is a function that handles a http request.
type RequestHandler func(w http.ResponseWriter, r *http.Request)

// leaderEngine is an interface to get the leader status.
type leaderEngine interface {
	IsLeader() bool
	GetLeaderIP() (string, error)
}

type leaderForwarder interface {
	SetLeaderIP(ip string)
	Forward(w http.ResponseWriter, r *http.Request)
}

// LeaderProxyHandler forwards queries to the leader.
type LeaderProxyHandler struct {
	// handlerName is used for telemetry
	handlerName string
	// le is the leader election engine
	le leaderEngine
	// leaderforwarder is used to forward queries to the leader
	leaderForwarder leaderForwarder
	// leaderElectionEnabled is true if leader election is enabled
	leaderElectionEnabled bool
	// preHandler is always called and should be used to validate the request before forwarding it or handling it
	preHandler RequestPreHandler
	// leaderHandler is called only by the leader
	leaderHandler RequestHandler
}

// WithLeaderProxyHandler returns a http handler function that emits telemetry.
func WithLeaderProxyHandler(handlerName string, preHandler RequestPreHandler, leaderHandler RequestHandler) RequestHandler {
	leaderEngine, _ := leaderelection.GetLeaderEngine()
	lph := LeaderProxyHandler{
		handlerName:           handlerName,
		leaderForwarder:       GetGlobalLeaderForwarder(),
		le:                    leaderEngine,
		leaderElectionEnabled: config.Datadog.GetBool("leader_election"),
		preHandler:            preHandler,
		leaderHandler:         leaderHandler,
	}

	return lph.handle
}

func (lph *LeaderProxyHandler) handle(w http.ResponseWriter, r *http.Request) {
	// Perform some checks on the request to verify that it should be processed
	if shouldBeProcessed := lph.preHandler(w, r); !shouldBeProcessed {
		return
	}
	// As follower, forward to leader
	if lph.rejectOrForwardLeaderQuery(w, r) {
		return
	}

	// As leader, handle the request
	lph.leaderHandler(w, r)
}

// rejectOrForwardLeaderQuery performs some checks on incoming queries that should go to a leader:
// - Forward to leader if we are a follower
// - Reject with "not ready" if leader election status is unknown
func (lph *LeaderProxyHandler) rejectOrForwardLeaderQuery(rw http.ResponseWriter, req *http.Request) bool {
	if !lph.leaderElectionEnabled {
		return false
	}

	if lph.le == nil {
		leaderEngine, err := leaderelection.GetLeaderEngine()
		if err != nil {
			log.Errorf("leader engine can't be retrieved: %v", err)
			http.Error(rw, "leader engine can't be retrieved", http.StatusServiceUnavailable)
			return true
		}
		lph.le = leaderEngine
	}

	if lph.le.IsLeader() {
		return false
	}

	ip, err := lph.le.GetLeaderIP()
	if err != nil {
		log.Errorf("failed to retrieve leader ip: %v", err)
		http.Error(rw, "failed to retrieve leader ip", http.StatusServiceUnavailable)
		return true
	}

	// if the leader forwarder is not set, we can't forward the request
	if lph.leaderForwarder == nil {
		log.Errorf("leader forwarder is not available")
		http.Error(rw, "leader forwarder is not available", http.StatusServiceUnavailable)
		return true
	}

	lph.leaderForwarder.SetLeaderIP(ip)
	forwardedRequest.Inc(lph.handlerName)
	lph.leaderForwarder.Forward(rw, req)
	return true
}
