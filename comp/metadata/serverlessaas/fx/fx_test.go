// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fx

import (
	"context"
	"os"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/agentmetadata"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	runnerfx "github.com/DataDog/datadog-agent/comp/metadata/runner/fx"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type recordingSerializer struct {
	serializer.MetricSerializer
	mu       sync.Mutex
	payloads []*agentmetadata.Payload
}

func (s *recordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, payload.(*agentmetadata.Payload))
	return nil
}

func (s *recordingSerializer) snapshot() []*agentmetadata.Payload {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.payloads)
}

type fakeHostname struct{ hostnameinterface.Component }

func (fakeHostname) Get(context.Context) (string, error) { return "", nil }
func (fakeHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{}, nil
}

func TestRunnerHonorsConfiguration(t *testing.T) {
	t.Setenv("DD_SERVERLESS_AAS_EXTENSION_METADATA_ENABLED", "")
	t.Setenv("DD_AAS_EXTENSION_VERSION", "")
	t.Setenv("DD_AAS_JAVA_EXTENSION_VERSION", "")
	t.Setenv("DD_AAS_DOTNET_EXTENSION_VERSION", "3.12.0")
	t.Setenv("DD_SERVICE", "checkout")
	t.Setenv("DD_VERSION", "2026.10")
	t.Setenv("FUNCTIONS_WORKER_RUNTIME", "")
	require.NoError(t, os.Unsetenv("FUNCTIONS_WORKER_RUNTIME"))
	t.Setenv("WEBSITE_STACK", "")
	t.Setenv("WEBSITE_NODE_DEFAULT_VERSION", "")
	t.Setenv("WEBSITE_SLOT_NAME", "Production")
	for _, tc := range []struct {
		name       string
		aas        bool
		metadata   bool
		inventory  bool
		identity   bool
		wantReport bool
	}{
		{"metadata enabled", true, true, true, true, true},
		{"AAS disabled by default", false, true, true, true, false},
		{"extension defaults", true, false, true, true, false},
		{"inventory disabled", true, true, false, true, false},
		{"missing identity", true, true, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("WEBSITE_SITE_NAME", "")
			if tc.identity {
				t.Setenv("WEBSITE_SITE_NAME", "My-App")
			}
			t.Setenv("WEBSITE_OWNER_NAME", "sub-123+East US")
			t.Setenv("WEBSITE_RESOURCE_GROUP", "My-RG")
			t.Setenv("REGION_NAME", "East US")
			synctest.Test(t, func(t *testing.T) {
				settings := map[string]any{
					"enable_metadata_collection":        tc.metadata,
					"inventories_enabled":               tc.inventory,
					"inventories_configuration_enabled": false,
					"inventories_first_run_delay":       3600,
					"inventories_min_interval":          60,
					"inventories_max_interval":          600,
					"env":                               "staging",
					"site":                              "datad0g.com",
				}
				if tc.aas {
					settings["serverless.aas_extension_metadata_enabled"] = true
				}
				conf := config.NewMockWithOverrides(t, settings)
				s := &recordingSerializer{}
				var rootConfig config.Component
				app := fx.New(
					fx.NopLogger,
					fxutil.FxLifecycleAdapter(),
					fx.Provide(func() config.Component { return conf }),
					fx.Provide(func() log.Component { return logmock.New(t) }),
					fx.Provide(func() hostnameinterface.Component { return fakeHostname{} }),
					fx.Provide(func() serializer.MetricSerializer { return s }),
					Module(),
					runnerfx.Module(),
					fx.Populate(&rootConfig),
					fx.Invoke(func(_ runner.Component) {}),
					fx.Invoke(func(in struct {
						fx.In
						Providers []runner.MetadataProvider `group:"metadata_provider"`
					}) {
						expected := 0
						if tc.aas && tc.metadata && tc.inventory {
							expected = 1
						}
						require.Len(t, runner.GetAndFilterProviders(in.Providers), expected)
					}),
				)
				require.NoError(t, app.Err())
				require.Empty(t, s.snapshot(), "construction must not submit")
				if tc.wantReport {
					t.Setenv("WEBSITE_SITE_NAME", "changed-after-construction")
					t.Setenv("REGION_NAME", "changed-after-construction")
				}
				require.NoError(t, app.Start(t.Context()))
				synctest.Wait()
				require.Equal(t, tc.metadata, rootConfig.GetBool("enable_metadata_collection"), "the component must not override configuration")
				require.Equal(t, tc.inventory, rootConfig.GetBool("inventories_enabled"))
				if tc.wantReport {
					payloads := s.snapshot()
					require.Len(t, payloads, 1)
					firstPayload := payloads[0]
					require.NotEmpty(t, firstPayload.UUID)
					require.Empty(t, firstPayload.Hostname)
					require.NotEmpty(t, firstPayload.Metadata["agent_version"])
					for key, expected := range map[string]any{
						"flavor":                "serverless-extension",
						"report_reason":         "startup",
						"resource_id":           "/subscriptions/sub-123/resourcegroups/my-rg/providers/microsoft.web/sites/my-app",
						"resource_name":         "My-App",
						"workload_type":         "azure_app_service",
						"region":                "East US",
						"azure_subscription_id": "sub-123",
						"azure_resource_group":  "My-RG",
						"runtime":               ".NET",
						"extension_version":     "3.12.0",
						"dd_env":                "staging",
						"dd_site":               "datad0g.com",
						"dd_service":            "checkout",
						"dd_version":            "2026.10",
					} {
						require.Equal(t, expected, firstPayload.Metadata[key], key)
					}
					time.Sleep(10 * time.Minute) // Advance virtual time to the next report.
					synctest.Wait()
					payloads = s.snapshot()
					require.Len(t, payloads, 2)
					require.Equal(t, "periodic", payloads[1].Metadata["report_reason"])
					require.Equal(t, "startup", firstPayload.Metadata["report_reason"], "later collections must not mutate queued payloads")
				} else {
					require.Empty(t, s.snapshot())
				}
				require.NoError(t, app.Stop(t.Context()))
				count := len(s.snapshot())
				time.Sleep(20 * time.Minute) // Virtual time: a stopped runner must not report again.
				synctest.Wait()
				require.Len(t, s.snapshot(), count)
			})
		})
	}
}
