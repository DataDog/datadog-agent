// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package impl provides common Agent API endpoints implementation
package impl

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider provides the common Agent API endpoints
type Provider struct {
	VersionEndpoint  api.AgentEndpointProvider
	HostnameEndpoint api.AgentEndpointProvider
	StopEndpoint     api.AgentEndpointProvider
}

// CommonEndpointProvider return a filled Provider struct
func CommonEndpointProvider() Provider {
	return Provider{
		VersionEndpoint:  api.NewAgentEndpointProvider(common.GetVersion, "/version", "GET"),
		HostnameEndpoint: api.NewAgentEndpointProvider(getHostname, "/hostname", "GET"),
		StopEndpoint:     api.NewAgentEndpointProvider(stopAgent, "/stop", "POST"),
	}
}

// getHostname returns the hostname as a JSON response.
func getHostname(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	hname, err := hostname.Get(r.Context())
	if err != nil {
		log.Warnf("Error getting hostname: %s\n", err) // or something like this
		hname = ""
	}
	j, _ := json.Marshal(hname)
	w.Write(j)
}

// StopAgent stops the agent by sending a signal to the stopper channel.
func stopAgent(w http.ResponseWriter, _ *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}
