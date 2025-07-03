// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processcheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessChecksIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		configs map[string]interface{}
		enabled bool
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			enabled: true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": false,
			},
			enabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fxutil.Test[processcheck.Component](t, fx.Options(
				core.MockBundle(),
				fx.Replace(config.MockParams{Overrides: tc.configs}),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				gpusubscriberfxmock.MockModule(),
				fx.Provide(func() statsd.ClientInterface {
					return &statsd.NoOpClient{}
				}),
				fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
				Module(),
			))
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
