// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"

	configendpoint "github.com/DataDog/datadog-agent/comp/api/api/apiimpl/internal/config"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/apiimpl/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
)

const ipcServerName string = "IPC API Server"

var ipcListener net.Listener

func startIPCServer(ipcServerAddr string, tlsConfig *tls.Config) (err error) {
	ipcListener, err = getListener(ipcServerAddr)
	if err != nil {
		return err
	}

	configEndpointMux := configendpoint.GetConfigEndpointMuxCore()
	configEndpointMux.Use(validateToken)

	ipcMux := http.NewServeMux()
	ipcMux.Handle(
		"/config/v1/",
		http.StripPrefix("/config/v1", configEndpointMux))
	ipcMuxHandler := apiutils.LogResponseHandler(ipcServerName)(ipcMux)

	ipcServer := &http.Server{
		Addr:      ipcServerAddr,
		Handler:   http.TimeoutHandler(ipcMuxHandler, time.Duration(config.Datadog.GetInt64("server_timeout"))*time.Second, "timeout"),
		TLSConfig: tlsConfig,
	}

	startServer(ipcListener, ipcServer, ipcServerName)

	return nil
}

func stopIPCServer() {
	stopServer(ipcListener, ipcServerName)
}
