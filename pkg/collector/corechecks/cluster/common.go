// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package cluster

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
)

// RunLeaderElection runs the leader election engine and identifies the leader.
// It returns leader name and a nil error if leader.
// It returns leader name and a ErrNotLeader error if not leader.
func RunLeaderElection() (string, error) {
	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return "", err
	}

	err = leaderEngine.EnsureLeaderElectionRuns()
	if err != nil {
		return "", err
	}

	if !leaderEngine.IsLeader() {
		return leaderEngine.GetLeader(), apiserver.ErrNotLeader
	}

	return leaderEngine.GetLeader(), nil
}
