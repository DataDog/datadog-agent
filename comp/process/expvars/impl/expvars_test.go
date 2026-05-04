// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package expvarsimpl_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	mocktelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	expvars "github.com/DataDog/datadog-agent/comp/process/expvars/def"
	expvarsfx "github.com/DataDog/datadog-agent/comp/process/expvars/fx"
	hostinfomock "github.com/DataDog/datadog-agent/comp/process/hostinfo/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestExpvarServer(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor("process_agent")

	_ = fxutil.Test[expvars.Component](t, fx.Options(
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"process_config.expvar_port": 43423,
			})
		}),
		mocktelemetry.Module(),
		sysprobeconfigimpl.MockModule(),
		hostinfomock.MockModule(),
		expvarsfx.Module(),
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:43423/debug/vars")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}

func TestTelemetry(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)
	flavor.SetFlavor("process_agent")

	_ = fxutil.Test[expvars.Component](t, fx.Options(
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"telemetry.enabled":          true,
				"process_config.expvar_port": 43423,
			})
		}),
		expvarsfx.Module(),
		hostinfomock.MockModule(),
		mocktelemetry.Module(),
		sysprobeconfigimpl.MockModule(),
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:43423/telemetry")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}
