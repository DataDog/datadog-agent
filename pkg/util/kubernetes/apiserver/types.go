// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package apiserver

// LeaderElectorInterface is the interface avoiding the import cycle between the LeaderElection and the APIServer
type LeaderElectorInterface interface {
	IsLeader() bool
}

const (
	autoscalerNowHandleMsgEvent = "Autoscaler is now handled by the Cluster-Agent"
)
