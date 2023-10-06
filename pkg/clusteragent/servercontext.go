// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusteragent contains the functionality of the Cluster Agent.
package clusteragent

import "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"

// ServerContext holds business logic classes required to setup API endpoints
type ServerContext struct {
	ClusterCheckHandler *clusterchecks.Handler
}
