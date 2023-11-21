// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"crypto/tls"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"strconv"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func startServer(listener net.Listener, srv *http.Server, name string) {
	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := config.NewLogWriter(5, seelog.ErrorLvl)

	srv.ErrorLog = stdLog.New(logWriter, fmt.Sprintf("Error from the agent http %s: ", name), 0) // log errors to seelog

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck

	log.Infof("%s started on %s", name, listener.Addr().String())
}

// StartServer creates the router and starts the HTTP server
func StartServer(
	configService *remoteconfig.Service,
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
) error {
	apiAddr, err := getIPCAddressPort()
	if err != nil {
		panic("unable to get IPC address and port")
	}

	extraHosts := []string{apiAddr}
	ipcHost, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigPort := config.Datadog.GetInt("api_config_port")
	apiConfigEnabled := apiConfigPort > 0
	apiConfigHostPort := net.JoinHostPort(ipcHost, strconv.Itoa(apiConfigPort))

	if apiConfigEnabled {
		extraHosts = append(extraHosts, apiConfigHostPort)
	}

	tlsKeyPair, tlsCertPool := initializeTLS(extraHosts...)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsKeyPair},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}

	if err := util.CreateAndSetAuthToken(); err != nil {
		return err
	}

	if err := startAPIServer(
		apiAddr, tlsConfig, tlsCertPool,
		configService, flare, dogstatsdServer,
		capture, serverDebug, wmeta, logsAgent,
		senderManager, hostMetadata, invAgent,
	); err != nil {
		return err
	}

	if apiConfigEnabled {
		if err := startConfigServer(apiConfigHostPort, tlsConfig); err != nil {
			StopServer()
			return err
		}
	}

	return nil
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if apiListener != nil {
		apiListener.Close()
	}
	if apiConfigListener != nil {
		apiConfigListener.Close()
	}
}
