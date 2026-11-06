// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package run

import (
	"context"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	configstreamtestutil "github.com/DataDog/datadog-agent/comp/core/configstreamconsumer/testutil"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/configstreambootstrap"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/credentials/resolver"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestPrivateActionRunnerRunCommand(t *testing.T) {
	t.Setenv("DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED", "false")
	t.Run("disabled", func(t *testing.T) {
		// Test when PAR is disabled - should exit cleanly without calling fxutil.Run
		commands := Commands(newGlobalParamsTest(t, false))
		err := commands[0].RunE(nil, []string{"run"})
		require.NoError(t, err)
	})

	t.Run("enabled", func(t *testing.T) {
		// Test when PAR is enabled - should call fxutil.Run
		fxutil.TestRun(t, func() error {
			commands := Commands(newGlobalParamsTest(t, true))
			return commands[0].RunE(nil, []string{"run"})
		})
	})
}

func TestStreamedCredentials(t *testing.T) {
	configstreambootstrap.ResetGlobalConfig(t)
	configstreambootstrap.UseDynamicSchema(t)
	t.Setenv("DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED", "true")
	credentialValues := func(value string) map[string]any {
		return map[string]any{"database_password": map[string]any{"value": value}}
	}
	fakeCore := configstreamtestutil.StartFakeCoreAgent(t, t.TempDir(),
		configstreamtestutil.WithSnapshotSettings(map[string]any{par.CredentialsValues: credentialValues("startup")}))
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	// Local secret resolution would fail; the consumer must use core's resolved snapshot.
	require.NoError(t, os.WriteFile(configPath, []byte(fakeCore.ConfigYAML()+`
secret_backend_command: /does/not/exist
private_action_runner:
  credentials:
    values:
      database_password:
        value: ENC[database_password]
`), 0600))
	deps := fxutil.Test[struct {
		fx.In
		Config config.Component
	}](t,
		core.Bundle(core.WithSecrets()),
		fx.Replace(fx.Annotate(telemetryimpl.NewMock(t), fx.As(new(telemetry.Component)))),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(configPath),
			LogParams:    log.ForOneShot(command.LoggerName, "off", false),
		}),
		configStreamOptions(configPath),
	)
	credentialResolver, err := resolver.NewPrivateCredentialResolver(deps.Config)
	require.NoError(t, err)
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{ConnectionInfo: &privateactionspb.ConnectionInfo{
		CredentialsType: privateactionspb.CredentialsType_CONNECTION_TOKENS_V2,
		TokensV2: []*privateactionspb.ConnectionTokenV2{{
			NameSegments: []string{"root_tokens", "password"},
			Source: &privateactionspb.ConnectionTokenV2_RunnerCredential_{
				RunnerCredential: &privateactionspb.ConnectionTokenV2_RunnerCredential{Key: "database_password"},
			},
		}},
	}}
	credentials, err := credentialResolver.ResolveCredentials(context.Background(), task)
	require.NoError(t, err)
	require.Len(t, credentials.Tokens, 1)
	require.Equal(t, "startup", credentials.Tokens[0].Value)

	rotated, err := structpb.NewValue(credentialValues("rotated"))
	require.NoError(t, err)
	fakeCore.Events <- &pb.ConfigEvent{Event: &pb.ConfigEvent_Update{Update: &pb.ConfigUpdate{
		SequenceId: 2,
		Setting:    &pb.ConfigSetting{Key: par.CredentialsValues, Value: rotated, Source: "secret"},
	}}}
	require.Eventually(t, func() bool {
		credentials, err := credentialResolver.ResolveCredentials(context.Background(), task)
		return err == nil && len(credentials.Tokens) == 1 && credentials.Tokens[0].Value == "rotated"
	}, 5*time.Second, 10*time.Millisecond)
}

func newGlobalParamsTest(t *testing.T, enabled bool) *command.GlobalParams {
	// Create minimal config for private action runner testing
	configPath := path.Join(t.TempDir(), "datadog.yaml")
	configContent := `
hostname: test
private_action_runner:
  enabled: %v
  private_key: test_private_key
  urn: test_urn
api_key: test_key
`
	err := os.WriteFile(configPath, []byte(fmt.Sprintf(configContent, enabled)), 0644)
	require.NoError(t, err)

	return &command.GlobalParams{
		ConfFilePath: configPath,
	}
}
