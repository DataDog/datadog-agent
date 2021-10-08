package http

import (
	"encoding/json"
	"html"
	"net/http"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

// Server offers functions that implement the standard runtime settings HTTP API
var Server = struct {
	GetFull          func(string) http.HandlerFunc
	GetValue         http.HandlerFunc
	SetValue         http.HandlerFunc
	ListConfigurable http.HandlerFunc
}{
	GetFull:          getFullConfig,
	GetValue:         getConfigValue,
	SetValue:         setConfigValue,
	ListConfigurable: listConfigurableSettings,
}

func getFullConfig(namespace string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		var nsSettings interface{}
		allSettings := ddconfig.Datadog.AllSettings()
		if namespace != "" {
			for k, v := range allSettings {
				if k == namespace {
					nsSettings = v
					break
				}
			}
		} else {
			nsSettings = allSettings
		}

		runtimeConfig, err := yaml.Marshal(nsSettings)
		if err != nil {
			log.Errorf("Unable to marshal runtime config response: %s", err)
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusInternalServerError)
			return
		}

		scrubbed, err := log.CredentialsCleanerBytes(runtimeConfig)
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
	body, err := json.Marshal(map[string]interface{}{"value": val})
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

	if err := settings.SetRuntimeSetting(setting, value); err != nil {
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
