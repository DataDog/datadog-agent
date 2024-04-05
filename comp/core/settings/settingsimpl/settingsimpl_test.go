// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settingsimpl

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
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

func (t *runtimeTestSetting) Get() (interface{}, error) {
	return returnValue{
		Value:  t.value,
		Source: t.source,
	}, nil
}

func (t *runtimeTestSetting) Set(v interface{}, source model.Source) error {
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

				comp.GetFullConfig(config.Datadog, "")(responseRecorder, request)
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
			"ListConfigurable",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()
				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.ListConfigurable(responseRecorder, request)
				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				assert.Equal(t, "{\"bar\":{\"Description\":\"bar settings\",\"Hidden\":false},\"foo\":{\"Description\":\"foo settings\",\"Hidden\":false},\"hidden setting\":{\"Description\":\"hidden setting\",\"Hidden\":true}}", string(body))
			},
		},
		{
			"GetValue",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()

				request := httptest.NewRequest("GET", "http://agent.host/test/", nil)

				comp.GetValue("foo", responseRecorder, request)

				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				assert.Equal(t, "{\"value\":{\"Value\":\"\",\"Source\":\"\"}}", string(body))

				requestWithSources := httptest.NewRequest("GET", "http://agent.host?sources=true", nil)
				responseRecorder2 := httptest.NewRecorder()

				comp.GetValue("foo", responseRecorder2, requestWithSources)
				resp2 := responseRecorder2.Result()
				defer resp2.Body.Close()
				body, _ = io.ReadAll(resp2.Body)

				assert.Equal(t, 200, responseRecorder2.Code)
				assert.Equal(t, "{\"sources_value\":[{\"Source\":\"default\",\"Value\":null},{\"Source\":\"unknown\",\"Value\":null},{\"Source\":\"file\",\"Value\":null},{\"Source\":\"environment-variable\",\"Value\":null},{\"Source\":\"agent-runtime\",\"Value\":null},{\"Source\":\"local-config-process\",\"Value\":null},{\"Source\":\"remote-config\",\"Value\":null},{\"Source\":\"cli\",\"Value\":null}],\"value\":{\"Value\":\"\",\"Source\":\"\"}}", string(body))

				responseRecorder3 := httptest.NewRecorder()

				comp.GetValue("non_existing", responseRecorder3, request)
				resp3 := responseRecorder3.Result()
				defer resp3.Body.Close()
				body, _ = io.ReadAll(resp3.Body)

				assert.Equal(t, 400, responseRecorder3.Code)
				assert.Equal(t, "{\"error\":\"setting non_existing not found\"}\n", string(body))
			},
		},
		{
			"SetValue",
			func(t *testing.T, comp settings.Component) {
				responseRecorder := httptest.NewRecorder()

				requestBody := fmt.Sprintf("value=%s", html.EscapeString("fancy"))
				request := httptest.NewRequest("POST", "http://agent.host/test/", bytes.NewBuffer([]byte(requestBody)))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				comp.SetValue("foo", responseRecorder, request)

				resp := responseRecorder.Result()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				assert.Equal(t, 200, responseRecorder.Code)
				assert.Equal(t, "", string(body))

				responseRecorder2 := httptest.NewRecorder()

				comp.GetValue("foo", responseRecorder2, request)
				resp2 := responseRecorder2.Result()
				defer resp2.Body.Close()
				body, _ = io.ReadAll(resp2.Body)

				assert.Equal(t, 200, responseRecorder2.Code)
				assert.Equal(t, "{\"value\":{\"Value\":\"fancy\",\"Source\":\"cli\"}}", string(body))
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			deps := fxutil.Test[dependencies](t, fx.Options(
				logimpl.MockModule(),
				fx.Supply(
					settings.Settings{
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
				),
			))

			provides := newSettings(deps)
			settingsComponent := provides.Comp

			testCase.assertFunc(t, settingsComponent)
		})
	}
}
