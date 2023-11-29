// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const ipc_server_name string = "IPC API Server"

var ipcListener net.Listener
var allowedConfigPaths = map[string]struct{}{
	"api_key": {},
}

func startIPCServer(ipcServerAddr string, tlsConfig *tls.Config) (err error) {
	ipcListener, err = getListener(ipcServerAddr)
	if err != nil {
		return err
	}

	ipcMux := http.NewServeMux()
	ipcMux.Handle(
		"/config/",
		http.StripPrefix("/config", getConfigEndpointMux()))

	ipcServer := &http.Server{
		Addr:      ipcServerAddr,
		Handler:   http.TimeoutHandler(ipcMux, time.Duration(config.Datadog.GetInt64("server_timeout"))*time.Second, "timeout"),
		TLSConfig: tlsConfig,
	}

	startServer(ipcListener, ipcServer, ipc_server_name)

	return nil
}

func getConfigEndpointMux() *gorilla.Router {
	configEndpointHandler := func(w http.ResponseWriter, r *http.Request) {
		body, err := getConfigValueAsJSON(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(body)
	}

	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", configEndpointHandler).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", configEndpointHandler).Methods("GET")
	configEndpointMux.Use(validateToken)

	return configEndpointMux
}

func getConfigValueAsJSON(r *http.Request) ([]byte, error) {
	vars := gorilla.Vars(r)
	path := vars["path"]

	if _, ok := allowedConfigPaths[path]; !ok {
		log.Warn("config endpoint received a request from '%s' for config '%s' which is not allowed", r.RemoteAddr, path)
		return nil, fmt.Errorf("querying config value '%s' is not allowed", path)
	}

	log.Debug("config endpoint received a request from '%s' for config '%s'", r.RemoteAddr, path)
	value := config.Datadog.Get(path)
	if value == nil {
		return nil, fmt.Errorf("no runtime setting found for %s", path)
	}

	return json.Marshal(value)
}

func stopIPCServer() {
	stopServer(ipcListener, ipc_server_name)
}
