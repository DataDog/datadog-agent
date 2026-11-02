// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package start

import (
	"reflect"
	"strings"
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
	for _, tc := range []struct {
		name      string
		extension string
		startFunc interface{}
	}{
		{"host", "0", start},
		{"AAS", "1", startAAS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(serverlessenv.AzureAppServicesEnvVar, tc.extension)
			fxutil.TestOneShotSubcommand(t,
				[]*cobra.Command{MakeCommand("defaultLogFile")},
				[]string{"start", "--cfgpath", "PATH"},
				tc.startFunc,
				func(cliParams *CLIParams, _ config.Params) {
					require.Equal(t, "PATH", cliParams.confPath)
				})
		})
	}
}

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
			var startFunc interface{} = start
			wantMetadataDependencies := 5 // Runner plus the four regular providers.
			if tc.extension == "1" {
				startFunc = startAAS
				wantMetadataDependencies = 2 // Runner and the AAS provider only.
			}
			// Resolve the actual startup's metadata dependencies without running its
			// blocking body or constructing unrelated DogStatsD components.
			startupType := reflect.TypeOf(startFunc)
			var metadataTypes []reflect.Type
			for i := 0; i < startupType.NumIn(); i++ {
				dependency := startupType.In(i)
				if strings.HasPrefix(dependency.PkgPath(), "github.com/DataDog/datadog-agent/comp/metadata/") {
					metadataTypes = append(metadataTypes, dependency)
				}
			}
			require.Len(t, metadataTypes, wantMetadataDependencies)
			captureStartup := reflect.MakeFunc(reflect.FuncOf(metadataTypes, nil, false), func([]reflect.Value) []reflect.Value {
				return nil
			}).Interface()
			var providers struct {
				fx.In
				Values []runner.MetadataProvider `group:"metadata_provider,soft"`
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
				fx.Invoke(captureStartup),
				fx.Populate(&providers),
			)
			require.NoError(t, app.Err())
			require.Len(t, runner.GetAndFilterProviders(providers.Values), tc.wantProviders)
		})
	}
}
