// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package settingsimpl implements the interface for the settings component
package settingsimpl

import (
	"encoding/json"
	"html"
	"net/http"
	"sync"

	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSettings),
	)
}

type provides struct {
	fx.Out

	Comp settings.Component
}

type dependencies struct {
	fx.In

	Log      log.Component
	Settings settings.Settings
}

type settingsRegistry struct {
	rwMutex  sync.RWMutex
	settings settings.Settings
	log      log.Component
}

// RuntimeSettings returns all runtime configurable settings
func (s *settingsRegistry) RuntimeSettings() settings.Settings {
	s.rwMutex.RLock()
	defer s.rwMutex.RUnlock()
	settingsCopy := settings.Settings{}
	for k, v := range s.settings {
		settingsCopy[k] = v
	}
	return settingsCopy
}

// GetRuntimeSetting returns the value of a runtime configurable setting
func (s *settingsRegistry) GetRuntimeSetting(setting string) (interface{}, error) {
	s.rwMutex.RLock()
	defer s.rwMutex.RUnlock()
	if _, ok := s.settings[setting]; !ok {
		return nil, &settings.SettingNotFoundError{Name: setting}
	}
	return s.settings[setting].Get()
}

// SetRuntimeSetting changes the value of a runtime configurable setting
func (s *settingsRegistry) SetRuntimeSetting(setting string, value interface{}, source model.Source) error {
	s.rwMutex.Lock()
	defer s.rwMutex.Unlock()
	if _, ok := s.settings[setting]; !ok {
		return &settings.SettingNotFoundError{Name: setting}
	}
	return s.settings[setting].Set(value, source)
}

func (s *settingsRegistry) GetFullConfig(cfg config.Config, namespaces ...string) http.HandlerFunc {
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

func (s *settingsRegistry) ListConfigurable(w http.ResponseWriter, _ *http.Request) {
	configurableSettings := make(map[string]settings.RuntimeSettingResponse)
	for name, setting := range s.RuntimeSettings() {
		configurableSettings[name] = settings.RuntimeSettingResponse{
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

func (s *settingsRegistry) GetValue(setting string, w http.ResponseWriter, r *http.Request) {
	s.log.Infof("Got a request to read a setting value: %s", setting)

	val, err := s.GetRuntimeSetting(setting)
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
		s.log.Errorf("Unable to marshal runtime setting value response: %s", err)
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		http.Error(w, string(body), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(body)
}

func (s *settingsRegistry) SetValue(setting string, w http.ResponseWriter, r *http.Request) {
	s.log.Infof("Got a request to change a setting: %s", setting)
	_ = r.ParseForm()
	value := html.UnescapeString(r.Form.Get("value"))

	if err := s.SetRuntimeSetting(setting, value, model.SourceCLI); err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		switch err.(type) {
		case *settings.SettingNotFoundError:
			http.Error(w, string(body), http.StatusBadRequest)
		default:
			http.Error(w, string(body), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func newSettings(deps dependencies) provides {
	return provides{
		Comp: &settingsRegistry{
			settings: deps.Settings,
			log:      deps.Log,
		},
	}
}
