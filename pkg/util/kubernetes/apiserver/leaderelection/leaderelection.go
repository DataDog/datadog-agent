// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package leaderelection provides functions related with the leader election
// mechanism offered in Kubernetes.
package leaderelection

import (
	"context"
)

// LeaderEngine is a structure for the LeaderEngine client to run leader election
// on Kubernetes clusters
type LeaderEngine struct {
}

func newLeaderEngine(ctx context.Context) *LeaderEngine {
	return &LeaderEngine{}
}

// ResetGlobalLeaderEngine is a helper to remove the current LeaderEngine global
// It is ONLY to be used for tests
func ResetGlobalLeaderEngine() {
}

// GetLeaderEngine returns an initialized leader engine.
func GetLeaderEngine() (*LeaderEngine, error) {
	return nil, nil
}

// CreateGlobalLeaderEngine returns a non initialized leader engine client
func CreateGlobalLeaderEngine(ctx context.Context) *LeaderEngine {
	return nil
}

func (le *LeaderEngine) init() error {
	return nil
}

// StartLeaderElectionRun starts the runLeaderElection once
func (le *LeaderEngine) StartLeaderElectionRun() {
	/*le.once.Do(
		func() {
			go le.runLeaderElection()
		},
	)
	*/
}

// EnsureLeaderElectionRuns start the Leader election process if not already running,
// return nil if the process is effectively running
func (le *LeaderEngine) EnsureLeaderElectionRuns() error {
	return nil
}

// GetLeader returns the identity of the last observed leader or returns the empty string if
// no leader has yet been observed.
func (le *LeaderEngine) GetLeader() string {
	return ""
}

// GetLeaderIP returns the IP the leader can be reached at, assuming its
// identity is its pod name. Returns empty if we are the leader.
// The result is cached and will not return an error if the leader does not exist anymore.
func (le *LeaderEngine) GetLeaderIP() (string, error) {
	return "", nil
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (le *LeaderEngine) IsLeader() bool {
	return false
}

// Subscribe allows any component to receive a notification when leadership state of the current
// process changes.
//
// The subscriber will not be notified about the leadership state change if the previous notification
// hasn't yet been consumed from the notification channel.
//
// Calling Subscribe is optional, use IsLeader if the client doesn't need an event-based approach.
func (le *LeaderEngine) Subscribe() (leadershipChangeNotif <-chan struct{}, isLeader func() bool) {
	return
}
