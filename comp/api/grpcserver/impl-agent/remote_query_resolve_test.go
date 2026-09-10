// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	remotequeriesimpl "github.com/DataDog/datadog-agent/comp/remotequeries/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// resolveFakeCollector and resolveFakeCheck mirror the impl-package fakes: the
// smallest check.Check surface plus the remote-query stream runner whose resolve
// verdicts the integration-owned sweep classifies (name, loader, config provider,
// instance config, resolve events).
type resolveFakeCollector struct {
	checks []check.Check
}

func (f resolveFakeCollector) GetChecks() []check.Check { return f.checks }

type resolveFakeCheck struct {
	name     string
	loader   string
	provider string
	instance string
	// resolveEvents mirrors the per-check resolve_target verdict: a final MATCHED
	// with the sanitized identity, or an error with code target_not_found.
	resolveEvents []check.RemoteQueryStreamEvent
	resolveErr    error
	resolveCalls  int
}

func (f *resolveFakeCheck) RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error {
	if integration != f.name {
		return assert.AnError
	}
	f.resolveCalls++
	if f.resolveErr != nil {
		return f.resolveErr
	}
	for _, event := range f.resolveEvents {
		if err := emit(event); err != nil {
			return err
		}
	}
	return nil
}

func (f resolveFakeCheck) Run() error { return nil }
func (f resolveFakeCheck) Stop()      {}
func (f resolveFakeCheck) Cancel()    {}
func (f resolveFakeCheck) String() string {
	return f.name
}
func (f resolveFakeCheck) Loader() string { return f.loader }
func (f resolveFakeCheck) Configure(sender.SenderManager, uint64, autodiscovery.Data, autodiscovery.Data, string, string) error {
	return nil
}
func (f resolveFakeCheck) Interval() time.Duration { return 0 }
func (f resolveFakeCheck) ID() checkid.ID          { return checkid.ID(f.name) }
func (f resolveFakeCheck) GetWarnings() []error    { return nil }
func (f resolveFakeCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.SenderStats{}, nil
}
func (f resolveFakeCheck) Version() string          { return "" }
func (f resolveFakeCheck) ConfigSource() string     { return "" }
func (f resolveFakeCheck) ConfigProvider() string   { return f.provider }
func (f resolveFakeCheck) IsTelemetryEnabled() bool { return false }
func (f resolveFakeCheck) InitConfig() string       { return "" }
func (f resolveFakeCheck) InstanceConfig() string   { return f.instance }
func (f resolveFakeCheck) GetDiagnoses() ([]diagnose.Diagnosis, error) {
	return nil, nil
}
func (f resolveFakeCheck) IsHASupported() bool { return false }

func TestRemoteQueryResolveReturnsResolutionErrorWhenServiceMissing(t *testing.T) {
	resp, err := (&serverSecure{}).RemoteQueryResolve(context.Background(), &pb.RemoteQueryResolveRequest{
		Integration: "postgres",
		Target:      &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, remotequeriesimpl.RemoteQueryStatusResolutionError, resp.GetStatus())
	assert.Equal(t, remotequeriesimpl.RemoteQueryStatusResolutionError, resp.GetErrorCode())
	assert.Equal(t, "remote query resolver is unavailable", resp.GetErrorMessage())
	assert.Empty(t, resp.GetMatchFingerprint())
}

// resolveMatchedEvents and resolveNotFoundEvents build the pinned per-check
// resolve_target verdicts the integration emits: one final MATCHED with the
// sanitized identity, or one existing-style error with code target_not_found.
func resolveMatchedEvents() []check.RemoteQueryStreamEvent {
	return []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"MATCHED","match":{"host":"localhost","port":5432,"configuredDbname":"postgres","resolvedDbname":"postgres"}}`}}
}

func resolveNotFoundEvents() []check.RemoteQueryStreamEvent {
	return []check.RemoteQueryStreamEvent{{Type: "error", MetadataJSON: `{"status":"FAILED","error":{"code":"target_not_found","message":"no loaded integration instance matched target selector."}}`}}
}

// TestRemoteQueryResolveAnswersStructuredOutcomes proves the AgentSecure resolve
// handler answers the contract statuses end to end through the integration-owned
// resolver sweep.
func TestRemoteQueryResolveAnswersStructuredOutcomes(t *testing.T) {
	server := &serverSecure{
		remoteQueriesResolve: remotequeriesimpl.NewRemoteQueryResolveService(resolveFakeCollector{checks: []check.Check{
			&resolveFakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\nusername: alice\npassword: secret-value\n", resolveEvents: resolveMatchedEvents()},
			&resolveFakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5433\ndbname: postgres\npassword: other-secret\n", resolveEvents: resolveNotFoundEvents()},
		}}, true),
	}

	t.Run("unique match answers matched with fingerprint", func(t *testing.T) {
		resp, err := server.RemoteQueryResolve(context.Background(), &pb.RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Host: "LOCALHOST.", Port: 5432, Dbname: "postgres"},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, remotequeriesimpl.RemoteQueryStatusMatched, resp.GetStatus())
		assert.Regexp(t, `^[0-9a-f]{64}$`, resp.GetMatchFingerprint())
		assert.Empty(t, resp.GetErrorCode())
		assert.Empty(t, resp.GetErrorMessage())
	})

	t.Run("zero matches answers target_not_found", func(t *testing.T) {
		noneServer := &serverSecure{
			remoteQueriesResolve: remotequeriesimpl.NewRemoteQueryResolveService(resolveFakeCollector{checks: []check.Check{
				&resolveFakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n", resolveEvents: resolveNotFoundEvents()},
			}}, true),
		}

		resp, err := noneServer.RemoteQueryResolve(context.Background(), &pb.RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Host: "nowhere", Port: 5432, Dbname: "other"},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "target_not_found", resp.GetStatus())
		assert.Equal(t, "target_not_found", resp.GetErrorCode())
		assert.Equal(t, "no matching integration check found", resp.GetErrorMessage())
		assert.Empty(t, resp.GetMatchFingerprint())
	})

	t.Run("multiple matches answer ambiguous_target", func(t *testing.T) {
		duplicateServer := &serverSecure{
			remoteQueriesResolve: remotequeriesimpl.NewRemoteQueryResolveService(resolveFakeCollector{checks: []check.Check{
				&resolveFakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n", resolveEvents: resolveMatchedEvents()},
				&resolveFakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n", resolveEvents: resolveMatchedEvents()},
			}}, true),
		}

		resp, err := duplicateServer.RemoteQueryResolve(context.Background(), &pb.RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "ambiguous_target", resp.GetStatus())
		assert.Equal(t, "ambiguous_target", resp.GetErrorCode())
		assert.Equal(t, "multiple matching integration checks found", resp.GetErrorMessage())
	})

	t.Run("malformed request answers resolution_error, not a miss", func(t *testing.T) {
		resp, err := server.RemoteQueryResolve(context.Background(), &pb.RemoteQueryResolveRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Port: 5432, Dbname: "postgres"},
		})

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, remotequeriesimpl.RemoteQueryStatusResolutionError, resp.GetStatus())
		assert.Equal(t, remotequeriesimpl.RemoteQueryStatusResolutionError, resp.GetErrorCode())
		assert.Equal(t, "target.host is required", resp.GetErrorMessage())
	})

	t.Run("nil request answers resolution_error", func(t *testing.T) {
		resp, err := server.RemoteQueryResolve(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, remotequeriesimpl.RemoteQueryStatusResolutionError, resp.GetStatus())
	})
}

// TestRemoteQueryResolveRequestFromProtoIsCredentialFree proves the resolve proto
// mapping carries only the integration and target fields.
func TestRemoteQueryResolveRequestFromProtoIsCredentialFree(t *testing.T) {
	req := remoteQueryResolveRequestFromProto(&pb.RemoteQueryResolveRequest{
		Integration: "postgres",
		Target:      &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres", DatabaseInstance: "ignored-instance"},
	})

	assert.Equal(t, "postgres", req.Integration)
	assert.Equal(t, remotequeriesimpl.RemoteQueryExecuteTarget{
		Host:             "localhost",
		Port:             5432,
		DBName:           "postgres",
		DatabaseInstance: "ignored-instance",
	}, req.Target)
}
