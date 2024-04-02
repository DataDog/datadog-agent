// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package serverimpl implements the interface for the settings server
package serverimpl

import (
	"encoding/json"
	"html"
	"net/http"

	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings/registry"
	"github.com/DataDog/datadog-agent/comp/core/settings/server"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/gorilla/mux"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newServer),
	)
}

type provides struct {
	fx.Out

	Comp server.Component
}

type dependencies struct {
	fx.In

	Log      log.Component
	Settings registry.Component
}

type settingsServer struct {
	settings registry.Component
	log      log.Component
}

func (s *settingsServer) GetFullConfig(cfg config.Config, namespaces ...string) http.HandlerFunc {
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
			s.log.Errorf("Unable to marshal runtime config response: %s", err)
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}

		scrubbed, err := scrubber.ScrubYaml(runtimeConfig)
		if err != nil {
			s.log.Errorf("Unable to scrub sensitive data from runtime config: %s", err)
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}

		_, _ = w.Write(scrubbed)
	}
}

func (s *settingsServer) ListConfigurable(w http.ResponseWriter, _ *http.Request) {
	configurableSettings := make(map[string]server.RuntimeSettingResponse)
	for name, setting := range s.settings.RuntimeSettings() {
		configurableSettings[name] = server.RuntimeSettingResponse{
			Description: setting.Description(),
			Hidden:      setting.Hidden(),
		}
	}
	body, err := json.Marshal(configurableSettings)
	if err != nil {
		s.log.Errorf("Unable to marshal runtime configurable settings list response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (s *settingsServer) GetValue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	setting := vars["setting"]
	s.log.Infof("Got a request to read a setting value: %s", setting)

	val, err := s.settings.GetRuntimeSetting(setting)
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *registry.SettingNotFoundError:
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
		s.log.Errorf("Unable to marshal runtime setting value response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (s *settingsServer) SetValue(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	setting := vars["setting"]
	s.log.Infof("Got a request to change a setting: %s", setting)
	_ = r.ParseForm()
	value := html.UnescapeString(r.Form.Get("value"))

	if err := s.settings.SetRuntimeSetting(setting, value, model.SourceCLI); err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *registry.SettingNotFoundError:
			http.Error(w, string(body), http.StatusBadRequest)
		default:
			http.Error(w, string(body), http.StatusInternalServerError)
		}
		return
	}
}

func newServer(deps dependencies) provides {
	return provides{
		Comp: &settingsServer{
			settings: deps.Settings,
		},
	}
}
