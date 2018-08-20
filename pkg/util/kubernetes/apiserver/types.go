// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

// LeaderElectorItf is the interface avoiding the import cycle between the LeaderElection and the APIServer
type LeaderElectorItf interface {
	//EnsureLeaderElectionRuns() error
	IsLeader() bool
}
