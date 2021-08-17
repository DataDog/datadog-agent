package api

import (
	"fmt"
	"net/http"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	settingshttp "github.com/DataDog/datadog-agent/pkg/config/settings/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/gorilla/mux"
)

// NOTE: Any settings you want to register should simply be added here
var processRuntimeSettings = []settings.RuntimeSetting{
	settings.LogLevelRuntimeSetting{},
}

// StartServer starts the config server
func StartServer() error {
	// Set up routes
	r := mux.NewRouter()
	r.HandleFunc("/config", settingshttp.Server.GetValue)
	r.HandleFunc("/config", settingshttp.Server.GetFull("")).Methods("GET")
	r.HandleFunc("/config/list-runtime", settingshttp.Server.ListConfigurable).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.GetValue).Methods("GET")
	r.HandleFunc("/config/{setting}", settingshttp.Server.SetValue).Methods("POST")

	addr, err := getIPCAddressPort()
	if err != nil {
		return err
	}
	log.Infof("Config server listening on %s", addr)

	srv := &http.Server{
		Handler: r,
		Addr:    addr,
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		_ = log.Warn(settings.RegisterRuntimeSetting(setting))
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
	return fmt.Sprintf("%v:%d", address, ddconfig.Datadog.GetInt("process_config.cmd_port")), nil
}
