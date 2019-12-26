// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

// LeaderElectorInterface is the interface avoiding the import cycle between the LeaderElection and the APIServer
type LeaderElectorInterface interface {
	IsLeader() bool
}

const (
	autoscalerNowHandleMsgEvent = "Autoscaler is now handle by the Cluster-Agent"
	autoscalerIgnoreMsgEvent    = "Autoscaler is ignored, to many metrics already handled by the Cluster-Agent"
	autoscalerUnIgnoreMsgEvent  = "Autoscaler will now be processed by the Cluster-Agent"
)
