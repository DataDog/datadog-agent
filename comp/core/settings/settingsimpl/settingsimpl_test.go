// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settingsimpl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type runtimeTestSetting struct {
	value       string
	source      model.Source
	hidden      bool
	description string
}

type returnValue struct {
	Value  string
	Source model.Source
}

func (t *runtimeTestSetting) Description() string {
	return t.description
}

func (t *runtimeTestSetting) Get(_ config.Component) (interface{}, error) {
	return returnValue{
		Value:  t.value,
		Source: t.source,
	}, nil
}

func (t *runtimeTestSetting) Set(_ config.Component, v interface{}, source model.Source) error {
	t.value = v.(string)
	t.source = source
	return nil
}

func (t *runtimeTestSetting) Hidden() bool {
	return t.hidden
}

func TestRuntimeSettings(t *testing.T) {
	testCases := []struct {
		name       string
		assertFunc func(t *testing.T, comp settings.Component)
	}{
		{
			"GetRuntimeSetting",
			func(t *testing.T, comp settings.Component) {
				_, err := comp.GetRuntimeSetting("foo")
				assert.NoError(t, err)

				_, err = comp.GetRuntimeSetting("non_existing")
				assert.Error(t, err)
			},
		},
		{
			"SetRuntimeSetting",
			func(t *testing.T, comp settings.Component) {
				value, err := comp.GetRuntimeSetting("foo")
				assert.NoError(t, err)

				result := value.(returnValue)

				assert.Equal(t, "", result.Value)
				assert.Equal(t, model.Source(""), result.Source)

				err = comp.SetRuntimeSetting("foo", "hello world", model.SourceDefault)
				assert.NoError(t, err)

				value, err = comp.GetRuntimeSetting("foo")
				assert.NoError(t, err)

				result = value.(returnValue)

				assert.Equal(t, "hello world", result.Value)
				assert.Equal(t, model.SourceDefault, result.Source)

				err = comp.SetRuntimeSetting("non_existing", "hello world", model.SourceDefault)
				assert.Error(t, err)
			},
		},
		{
			"GetFullConfig",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.GetFullConfig("")(responseRecorder, request)
				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				// The full config is too big to assert against
				// Ensure the response body is not empty to validate we wrote something
				assert.NotEqual(t, "", string(body))
			},
		},
		{
			"GetFullConfigBySource",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.GetFullConfigBySource()(responseRecorder, request)
				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				// The full config is too big to assert against
				// Ensure the response body is not empty to validate we wrote something
				assert.NotEqual(t, "", string(body))
			},
		},
		{
			"GetFullConfigBySource with http_replace_rules",
			func(t *testing.T, comp settings.Component) {
				// system-probe config contains []map[interface{}]interface{}, which 'encode/json'
				// cannot marshal. Using "github.com/json-iterator/go" instead fix the issue.
				//
				// This test verify that this is correctly handle byt settingsimpl.
				s := comp.(*settingsRegistry)
				s.config.Set(
					"service_monitoring_config.http_replace_rules",
					[]map[interface{}]interface{}{
						{
							"pattern": "/v\\d{1}\\.\\d{1}/traces",
						},
						{
							"pattern": "/v\\d{1}\\.\\d{1}/config",
						},
					},
					model.SourceUnknown)

				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.GetFullConfigBySource()(responseRecorder, request)
				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				// The full config is too big to assert against
				// Ensure the response body is not empty to validate we wrote something
				assert.NotEqual(t, "", string(body))
			},
		},
		{
			"GetFullConfigBySource with big config layers",
			func(t *testing.T, comp settings.Component) {
				layerMaxSize := 1024 * 60
				config := comp.(*settingsRegistry).config
				config.Set("big_config_value", strings.Repeat("a", layerMaxSize), model.SourceEnvVar)
				config.Set("big_config_value", strings.Repeat("b", layerMaxSize), model.SourceFile)
				config.Set("big_config_value", strings.Repeat("c", layerMaxSize), model.SourceAgentRuntime)

				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)
				comp.GetFullConfigBySource()(responseRecorder, request)

				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, 200, responseRecorder.Code)
				// The full config is too big to assert against
				// Ensure the response body is not empty to validate we wrote something
				assert.NotEqual(t, "", string(body))
			},
		},
		{
			"ListConfigurable",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.ListConfigurable(responseRecorder, request)
				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)

				// Order of the map is not guaranteed by "github.com/json-iterator/go" so we can't
				// simply compare strings.
				expected := map[string]interface{}{}
				actual := map[string]interface{}{}
				json.Unmarshal([]byte("{\"foo\":{\"Description\":\"foo settings\",\"Hidden\":false},\"hidden setting\":{\"Description\":\"hidden setting\",\"Hidden\":true},\"bar\":{\"Description\":\"bar settings\",\"Hidden\":false}}"), &expected)
				err := json.Unmarshal(body, &actual)

				require.NoError(t, err, fmt.Sprintf("error loading JSON body: %s", err))
				assert.Equal(t, expected, actual)
			},
		},
		{
			"GetValue",
			func(t *testing.T, comp settings.Component) {
				router := mux.NewRouter()
				router.HandleFunc("/config/{setting}", comp.GetValue).Methods("GET")
				ts := httptest.NewServer(router)
				defer ts.Close()

				request, err := http.NewRequest("GET", ts.URL+"/config/foo", nil)
				require.NoError(t, err)

				resp, err := ts.Client().Do(request)
				require.NoError(t, err)
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				assert.Equal(t, 200, resp.StatusCode)
				assert.Equal(t, "{\"value\":{\"Value\":\"\",\"Source\":\"\"}}", string(body))

				requestWithSources, err := http.NewRequest("GET", ts.URL+"/config/foo?sources=true", nil)
				require.NoError(t, err)

				resp, err = ts.Client().Do(requestWithSources)
				require.NoError(t, err)
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()

				assert.Equal(t, 200, resp.StatusCode)

				// Order of the map is not guaranteed by "github.com/json-iterator/go" so we can't
				// simply compare strings.
				expected := map[string]interface{}{}
				actual := map[string]interface{}{}
				json.Unmarshal([]byte("{\"value\":{\"Value\":\"\",\"Source\":\"\"},\"sources_value\":[{\"Source\":\"default\",\"Value\":null},{\"Source\":\"unknown\",\"Value\":null},{\"Source\":\"file\",\"Value\":null},{\"Source\":\"environment-variable\",\"Value\":null},{\"Source\":\"agent-runtime\",\"Value\":null},{\"Source\":\"local-config-process\",\"Value\":null},{\"Source\":\"remote-config\",\"Value\":null},{\"Source\":\"cli\",\"Value\":null}]}"), &expected)
				err = json.Unmarshal(body, &actual)

				require.NoError(t, err, fmt.Sprintf("error loading JSON body: %s", err))
				assert.Equal(t, expected, actual)

				unknownSettingRequest, err := http.NewRequest("GET", ts.URL+"/config/non_existing", nil)
				require.NoError(t, err)

				resp, err = ts.Client().Do(unknownSettingRequest)
				require.NoError(t, err)
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()

				assert.Equal(t, 400, resp.StatusCode)
				assert.Equal(t, "{\"error\":\"setting non_existing not found\"}\n", string(body))
			},
		},
		{
			"SetValue",
			func(t *testing.T, comp settings.Component) {
				router := mux.NewRouter()
				router.HandleFunc("/config/{setting}", comp.GetValue).Methods("GET")
				router.HandleFunc("/config/{setting}", comp.SetValue).Methods("POST")
				ts := httptest.NewServer(router)
				defer ts.Close()

				requestBody := fmt.Sprintf("value=%s", html.EscapeString("fancy"))
				request, err := http.NewRequest("POST", ts.URL+"/config/foo", bytes.NewBuffer([]byte(requestBody)))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				resp, err := ts.Client().Do(request)
				require.NoError(t, err)
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				assert.Equal(t, 200, resp.StatusCode)
				assert.Equal(t, "", string(body))

				request, err = http.NewRequest("GET", ts.URL+"/config/foo", nil)
				require.NoError(t, err)

				resp, err = ts.Client().Do(request)
				require.NoError(t, err)
				body, _ = io.ReadAll(resp.Body)
				resp.Body.Close()

				assert.Equal(t, 200, resp.StatusCode)
				assert.Equal(t, "{\"value\":{\"Value\":\"fancy\",\"Source\":\"cli\"}}", string(body))
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				fx.Supply(
					settings.Params{
						Config: fxutil.Test[config.Component](t, config.MockModule()),
						Settings: map[string]settings.RuntimeSetting{
							"foo": &runtimeTestSetting{
								hidden:      false,
								description: "foo settings",
							},
							"hidden setting": &runtimeTestSetting{
								hidden:      true,
								description: "hidden setting",
							},
							"bar": &runtimeTestSetting{
								hidden:      false,
								description: "bar settings",
							},
						},
					},
				),
			))

			provides := newSettings(deps)
			settingsComponent := provides.Comp

			testCase.assertFunc(t, settingsComponent)
		})
	}
}
