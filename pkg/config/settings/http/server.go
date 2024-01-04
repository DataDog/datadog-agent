// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"encoding/json"
	"html"
	"net/http"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Server offers functions that implement the standard runtime settings HTTP API
var Server = struct {
	GetFullDatadogConfig     func(...string) http.HandlerFunc
	GetFullSystemProbeConfig func(...string) http.HandlerFunc
	GetValue                 http.HandlerFunc
	GetValueWithSources      http.HandlerFunc
	SetValue                 http.HandlerFunc
	ListConfigurable         http.HandlerFunc
}{
	GetFullDatadogConfig:     getGlobalFullConfig(config.Datadog),
	GetFullSystemProbeConfig: getGlobalFullConfig(config.SystemProbe),
	GetValue:                 getConfigValue,
	SetValue:                 setConfigValue,
	ListConfigurable:         listConfigurableSettings,
}

func getGlobalFullConfig(cfg config.Config) func(...string) http.HandlerFunc {
	return func(namespaces ...string) http.HandlerFunc {
		return getFullConfig(cfg, namespaces...)
	}
}

func getFullConfig(cfg config.Config, namespaces ...string) http.HandlerFunc {
	requiresUniqueNs := len(namespaces) == 1 && namespaces[0] != ""
	requiresAllNamespaces := len(namespaces) == 0

	// We want to create a unique list of namespaces.
	uniqueNamespaces := map[string]struct{}{}
	for _, k := range namespaces {
		uniqueNamespaces[k] = struct{}{}
		if k == "" {
			requiresAllNamespaces = true
			break
		}
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		nsSettings := map[string]interface{}{}
		allSettings := cfg.AllSettings()
		if !requiresAllNamespaces {
			for ns := range uniqueNamespaces {
				if val, ok := allSettings[ns]; ok {
					nsSettings[ns] = val
				}
			}
		}

		var runtimeConfig []byte
		var err error
		if requiresUniqueNs {
			// This special case is to respect previous behavior not to display
			// a yaml root entry with the name of the namespace.
			runtimeConfig, err = yaml.Marshal(nsSettings[namespaces[0]])
		} else if requiresAllNamespaces {
			runtimeConfig, err = yaml.Marshal(allSettings)
		} else {
			runtimeConfig, err = yaml.Marshal(nsSettings)
		}
		if err != nil {
			log.Errorf("Unable to marshal runtime config response: %s", err)
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}

		scrubbed, err := scrubber.ScrubYaml(runtimeConfig)
		if err != nil {
			log.Errorf("Unable to scrub sensitive data from runtime config: %s", err)
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}

		_, _ = w.Write(scrubbed)
	}
}

func listConfigurableSettings(w http.ResponseWriter, _ *http.Request) {
	configurableSettings := make(map[string]settings.RuntimeSettingResponse)
	for name, setting := range settings.RuntimeSettings() {
		configurableSettings[name] = settings.RuntimeSettingResponse{
			Description: setting.Description(),
			Hidden:      setting.Hidden(),
		}
	}
	body, err := json.Marshal(configurableSettings)
	if err != nil {
		log.Errorf("Unable to marshal runtime configurable settings list response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func getConfigValue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	setting := vars["setting"]
	log.Infof("Got a request to read a setting value: %s", setting)

	val, err := settings.GetRuntimeSetting(setting)
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *settings.SettingNotFoundError:
			http.Error(w, string(body), http.StatusBadRequest)
		default:
			http.Error(w, string(body), http.StatusInternalServerError)
		}
		return
	}

	resp := map[string]interface{}{"value": val}
	if r.URL.Query().Get("sources") == "true" {
		resp["sources_value"] = config.Datadog.GetAllSources(setting)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		log.Errorf("Unable to marshal runtime setting value response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func setConfigValue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	setting := vars["setting"]
	log.Infof("Got a request to change a setting: %s", setting)
	_ = r.ParseForm()
	value := html.UnescapeString(r.Form.Get("value"))

	if err := settings.SetRuntimeSetting(setting, value, model.SourceCLI); err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *settings.SettingNotFoundError:
			http.Error(w, string(body), http.StatusBadRequest)
		default:
			http.Error(w, string(body), http.StatusInternalServerError)
		}
		return
	}
}
