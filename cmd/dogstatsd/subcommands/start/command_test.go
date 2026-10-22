// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestStartCommand(t *testing.T) {
	for _, deployment := range []string{"0", "1"} {
		t.Run("aas_extension_"+deployment, func(t *testing.T) {
			t.Setenv(serverlessenv.AzureAppServicesEnvVar, deployment)
			fxutil.TestOneShotSubcommand(t,
				[]*cobra.Command{MakeCommand("defaultLogFile")},
				[]string{"start", "--cfgpath", "PATH"},
				start,
				func(cliParams *CLIParams, _ config.Params) {
					require.Equal(t, "PATH", cliParams.confPath)
				})
		})
	}
}

// Exercise the real provider graph at construction time. Merely validating the
// command's CLI parameters does not verify which metadata providers are built.
func TestMetadataModulesSelectProviders(t *testing.T) {
	for _, tc := range []struct {
		name          string
		extension     string
		enabled       bool
		wantProviders int
	}{
		{"host", "0", false, 3},
		{"host ignores AAS flag", "0", true, 3},
		{"extension disabled", "1", false, 0},
		{"extension enabled", "1", true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(serverlessenv.AzureAppServicesEnvVar, tc.extension)
			var providers struct {
				fx.In
				Values []runner.MetadataProvider `group:"metadata_provider"`
			}
			app := fx.New(
				fx.NopLogger,
				fxutil.FxLifecycleAdapter(),
				fx.Provide(func() config.Component {
					return config.NewMockWithOverrides(t, map[string]any{
						"serverless.aas_extension_metadata_enabled": tc.enabled,
						"enable_metadata_collection":                true,
						"inventories_enabled":                       true,
					})
				}),
				fx.Provide(func() log.Component { return logmock.New(t) }),
				hostnameimpl.MockModule(),
				fx.Provide(func() serializer.MetricSerializer { return nil }),
				fx.Provide(func() ipc.HTTPClient { return nil }),
				metadataModules(),
				fx.Invoke(func(_ runner.Component) {}),
				fx.Populate(&providers),
			)
			require.NoError(t, app.Err())
			require.Len(t, runner.GetAndFilterProviders(providers.Values), tc.wantProviders)
		})
	}
}
