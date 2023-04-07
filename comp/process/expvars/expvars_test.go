// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package expvars

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestExpvarServer(t *testing.T) {
	_ = fxutil.Test[Component](t, fx.Options(
		fx.Supply(core.BundleParams{}),

		Module,
		hostinfo.MockModule,
		core.MockBundle,
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:6062/debug/vars")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}

func TestTelemetry(t *testing.T) {
	_ = fxutil.Test[Component](t, fx.Options(
		fx.Supply(core.BundleParams{}),
		fx.Replace(config.MockParams{Overrides: map[string]interface{}{
			"telemetry.enabled": true,
		}}),

		Module,
		hostinfo.MockModule,
		core.MockBundle,
	))

	assert.Eventually(t, func() bool {
		res, err := http.Get("http://localhost:6062/telemetry")
		if err != nil {
			return false
		}
		defer res.Body.Close()

		return res.StatusCode == http.StatusOK
	}, 5*time.Second, time.Second)
}
