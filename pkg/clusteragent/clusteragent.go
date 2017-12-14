// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package clusteragent

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Agent represents a Cluster Agent.
// It is responsible for running checks that need only run once per cluster.
// It also exposes
type Agent struct {
	metricOut       chan<- *metrics.MetricSample
	eventout        chan<- metrics.Event
	serviceCheckOut chan<- metrics.ServiceCheck
}

// Run returns a running Cluster agent instance
func Run(mOut chan<- *metrics.MetricSample, eOut chan<- metrics.Event, scOut chan<- metrics.ServiceCheck) (*Agent, error) {
	a := Agent{
		metricOut:       mOut,
		eventout:        eOut,
		serviceCheckOut: scOut,
	}
	log.Infof("Datadog cluster agent is now running.")
	return &a, nil
}

// Stop the cluster agent
func (a *Agent) Stop() {
	api.StopServer()
}


