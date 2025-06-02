// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package impl provides common Agent API endpoints implementation
package impl

import (
	"encoding/json"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/pkg/api/version"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires is a struct that contains the components required by the common endpoints
type Requires struct {
	Hostname hostnameinterface.Component
}

// Provider provides the common Agent API endpoints
type Provider struct {
	VersionEndpoint  api.AgentEndpointProvider
	HostnameEndpoint api.AgentEndpointProvider
	StopEndpoint     api.AgentEndpointProvider
}

// CommonEndpointProvider return a filled Provider struct
func CommonEndpointProvider(requires Requires) Provider {
	return Provider{
		VersionEndpoint:  api.NewAgentEndpointProvider(version.Get, "/version", "GET"),
		HostnameEndpoint: api.NewAgentEndpointProvider(getHostname(requires.Hostname), "/hostname", "GET"),
		StopEndpoint:     api.NewAgentEndpointProvider(stopAgent, "/stop", "POST"),
	}
}

// getHostname returns an http handler that writes the hostname as a JSON response.
func getHostname(hostname hostnameinterface.Component) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		hname, err := hostname.Get(r.Context())
		if err != nil {
			log.Warnf("Error getting hostname: %s\n", err) // or something like this
			hname = ""
		}
		j, _ := json.Marshal(hname)
		w.Write(j)
	}
}

// StopAgent stops the agent by sending a signal to the stopper channel.
func stopAgent(w http.ResponseWriter, _ *http.Request) {
	signals.Stopper <- true
	w.Header().Set("Content-Type", "application/json")
	j, _ := json.Marshal("")
	w.Write(j)
}
