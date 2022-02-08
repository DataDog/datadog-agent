// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupHandlers(r *mux.Router) {
	r.HandleFunc("/config", settingshttp.Server.GetFull("process_config")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")
}

// StartServer starts the config server
func StartServer() error {
	// Set up routes
	r := mux.NewRouter()
	setupHandlers(r)

	addr, err := getIPCAddressPort()
	if err != nil {
		return err
	}
	log.Infof("API server listening on %s", addr)

	srv := &http.Server{
		Handler: r,
		Addr:    addr,
	}

	go func() {
		err := srv.ListenAndServe()
		if err != nil {
			_ = log.Error(err)
		}
	}()
	return nil
}

// getIPCAddressPort returns a listening connection
func getIPCAddressPort() (string, error) {
	address, err := ddconfig.GetIPCAddress()
	if err != nil {
		return "", err
	}

	port := ddconfig.Datadog.GetInt("process_config.cmd_port")
	if port <= 0 {
		log.Warnf("Invalid process_config.cmd_port -- %d, using default port %d", port, ddconfig.DefaultProcessCmdPort)
		port = ddconfig.DefaultProcessCmdPort
	}

	addrPort := net.JoinHostPort(address, strconv.Itoa(port))
	return addrPort, nil
}
