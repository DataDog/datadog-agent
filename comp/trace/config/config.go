// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"strings"
	"testing"

	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type dependencies struct {
	fx.In

	Config coreconfig.Component
	Params Params
}

// cfg implements the Component.
type cfg struct {
	// this component is currently implementing a thin wrapper around pkg/trace/config,
	// and uses globals in that package.
	*traceconfig.AgentConfig

	// coreConfig relates to the main agent config component
	coreConfig coreconfig.Component

	// warnings are the warnings generated during setup
	warnings *pkgconfig.Warnings
}

func newConfig(deps dependencies) (Component, error) {
	tracecfg, err := setupConfig(deps)
	if err != nil {
		return nil, err
	}

	return &cfg{
		AgentConfig: tracecfg,
		coreConfig:  deps.Config,
	}, nil
}

func (c *cfg) Warnings() *pkgconfig.Warnings {
	return c.warnings
}

func (c *cfg) Object() *traceconfig.AgentConfig {
	return c.AgentConfig
}

// SetHandler returns handler for runtime configuration changes.
func (c *cfg) SetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpError(w, http.StatusMethodNotAllowed, fmt.Errorf("%s method not allowed, only %s", req.Method, http.MethodPost))
			return
		}
		for key, values := range req.URL.Query() {
			if len(values) == 0 {
				continue
			}
			value := html.UnescapeString(values[len(values)-1])
			switch key {
			case "log_level":
				lvl := strings.ToLower(value)
				if lvl == "warning" {
					lvl = "warn"
				}
				if err := pkgconfig.ChangeLogLevel(lvl); err != nil {
					httpError(w, http.StatusInternalServerError, err)
					return
				}
				pkgconfig.Datadog.Set("log_level", lvl)
				log.Infof("Switched log level to %s", lvl)
			default:
				log.Infof("Unsupported config change requested (key: %q).", key)
			}
		}
	})
}

func httpError(w http.ResponseWriter, status int, err error) {
	http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), status)
}

func newMock(deps dependencies, t testing.TB) Component {
	// old := nil // TODO: get whatever old config we're using here
	// config.SystemProbe = config.NewConfig("mock", "XXXX", strings.NewReplacer())
	c := &cfg{
		warnings: &pkgconfig.Warnings{},
	}

	// Viper's `GetXxx` methods read environment variables at the time they are
	// called, if those names were passed explicitly to BindEnv*(), so we must
	// also strip all `DD_` environment variables for the duration of the test.
	oldEnv := os.Environ()
	for _, kv := range oldEnv {
		if strings.HasPrefix(kv, "DD_") {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Unsetenv(kvslice[0])
		}
	}
	t.Cleanup(func() {
		for _, kv := range oldEnv {
			kvslice := strings.SplitN(kv, "=", 2)
			os.Setenv(kvslice[0], kvslice[1])
		}
	})

	// swap the existing config back at the end of the test.
	// TODO: obviously not systemprobe; cleanup accordingly to trace-agent
	// t.Cleanup(func() { config.SystemProbe = old })

	return c
}
