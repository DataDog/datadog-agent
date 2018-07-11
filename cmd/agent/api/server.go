// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/cmd/agent/api/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/api/check"
	"github.com/DataDog/datadog-agent/pkg/api"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var (
	server *api.Server
)

// StartServer starts the API HTTPS server
func StartServer() error {
	// Generate the auth token for client requests
	err := util.SetAuthToken()
	if err != nil {
		return err
	}

	// Setup the underlying TCP transport
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", config.Datadog.GetInt("cmd_port")))
	if err != nil {
		return err
	}

	// Setup the HTTPS server
	tlsConfig, err := security.GenerateSelfSignedConfig(api.LocalhostHosts)
	if err != nil {
		return err
	}
	server = api.NewServer(listener, tlsConfig, api.DefaultTokenValidator)

	// Register IPC endpoints
	agent.SetupHandlers(server.Router().PathPrefix("/agent").Subrouter())
	check.SetupHandlers(server.Router().PathPrefix("/check").Subrouter())

	server.Start()
	return nil
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if server != nil {
		server.Stop()
	}
}
