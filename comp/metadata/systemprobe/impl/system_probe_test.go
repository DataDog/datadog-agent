// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package impl implements the systemprobe metadata providers interface
package impl

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenimpl "github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	configFetcher "github.com/DataDog/datadog-agent/pkg/config/fetcher"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func setupFecther(t *testing.T) {
	t.Cleanup(func() {
		fetchSystemProbeConfig = configFetcher.SystemProbeConfig
		fetchSystemProbeConfigBySource = configFetcher.SystemProbeConfigBySource
	})

	fetchSystemProbeConfig = func(_ model.Reader) (string, error) { return "full config", nil }
	fetchSystemProbeConfigBySource = func(_ model.Reader) (string, error) {
		data, err := json.Marshal(map[string]interface{}{
			string(model.SourceFile):               map[string]bool{"file": true},
			string(model.SourceEnvVar):             map[string]bool{"env": true},
			string(model.SourceAgentRuntime):       map[string]bool{"runtime": true},
			string(model.SourceLocalConfigProcess): map[string]bool{"local": true},
			string(model.SourceRC):                 map[string]bool{"rc": true},
			string(model.SourceCLI):                map[string]bool{"cli": true},
			string(model.SourceProvided):           map[string]bool{"provided": true},
		})
		return string(data), err
	}
}

func getSystemProbeComp(t *testing.T, enableConfig bool) *systemprobe {
	l := fxutil.Test[log.Component](t, logimpl.MockModule())

	cfg := fxutil.Test[config.Component](t, config.MockModule())
	cfg.Set("inventories_configuration_enabled", enableConfig, model.SourceUnknown)

	r := Requires{
		Log:        l,
		Config:     cfg,
		Serializer: &serializer.MockSerializer{},
		AuthToken: fxutil.Test[authtoken.Component](t,
			authtokenimpl.Module(),
			fx.Provide(func() log.Component { return l }),
			fx.Provide(func() config.Component { return cfg }),
		),
		SysProbeConfig: fxutil.Test[optional.Option[sysprobeconfig.Component]](t, sysprobeconfigimpl.MockModule()),
	}

	comp := NewComponent(r).Comp
	return comp.(*systemprobe)
}

func assertPayload(t *testing.T, p *Payload) {
	assert.Equal(t, "test hostname", p.Hostname)
	assert.True(t, p.Timestamp <= time.Now().UnixNano())
	assert.Equal(t,
		map[string]interface{}{
			"agent_runtime_configuration":        "runtime: true\n",
			"cli_configuration":                  "cli: true\n",
			"environment_variable_configuration": "env: true\n",
			"file_configuration":                 "file: true\n",
			"full_configuration":                 "full config",
			"provided_configuration":             "provided: true\n",
			"remote_configuration":               "rc: true\n",
			"source_local_configuration":         "local: true\n",
			"agent_version":                      version.AgentVersion,
		},
		p.Metadata)
}

func TestGetPayload(t *testing.T) {
	setupFecther(t)
	sb := getSystemProbeComp(t, true)

	sb.hostname = "test hostname"

	p := sb.getPayload().(*Payload)
	assertPayload(t, p)
}

func TestGetPayloadNoConfig(t *testing.T) {
	setupFecther(t)
	sb := getSystemProbeComp(t, false)

	sb.hostname = "test hostname"

	p := sb.getPayload().(*Payload)
	assert.Equal(t, "test hostname", p.Hostname)
	assert.True(t, p.Timestamp <= time.Now().UnixNano())
	assert.Equal(t,
		map[string]interface{}{
			"agent_version": version.AgentVersion,
		},
		p.Metadata)
}

func TestWritePayload(t *testing.T) {
	setupFecther(t)
	sb := getSystemProbeComp(t, true)

	sb.hostname = "test hostname"

	req := httptest.NewRequest("GET", "http://fake_url.com", nil)
	w := httptest.NewRecorder()

	sb.writePayloadAsJSON(w, req)

	resp := w.Result()
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(t, err)

	p := Payload{}
	err = json.Unmarshal(body, &p)
	require.NoError(t, err)

	assertPayload(t, &p)
}
