// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// scriptedFetcher is a ConfigFetcher that returns a pre-built response per poll:
// poll 1 -> script[0], poll 2 -> script[1], etc. Once the script is exhausted it
// repeats the last payload (real RC re-sends the current active set every poll;
// an empty payload would instead look like "all configs deleted").
type scriptedFetcher struct {
	script []*pbgo.ClientGetConfigsResponse
	calls  atomic.Int32
}

func (f *scriptedFetcher) ClientGetConfigs(_ context.Context, _ *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	n := int(f.calls.Add(1)) - 1
	if n < len(f.script) {
		return f.script[n], nil
	}
	return f.script[len(f.script)-1], nil
}

// harnessConfig describes one config file to embed in a scripted response.
type harnessConfig struct {
	product  string
	configID string
	version  uint64
	body     string
}

func (hc harnessConfig) path() string {
	// datadog/<org>/<PRODUCT>/<configID>/<name>
	return fmt.Sprintf("datadog/2/%s/%s/config", hc.product, hc.configID)
}

// buildResponse assembles a ClientGetConfigsResponse the unverified Update path
// accepts: a TUF "targets" blob (signatures are NOT checked without TUF
// verification, but per-file sha256 + length ARE), plus the raw bodies.
func buildResponse(targetsVersion uint64, configs ...harnessConfig) *pbgo.ClientGetConfigsResponse {
	targetEntries := map[string]any{}
	files := make([]*pbgo.File, 0, len(configs))
	clientConfigs := make([]string, 0, len(configs))

	for _, c := range configs {
		raw := []byte(c.body)
		sum := sha256.Sum256(raw)
		targetEntries[c.path()] = map[string]any{
			"length": len(raw),
			"hashes": map[string]string{"sha256": hex.EncodeToString(sum[:])},
			"custom": map[string]any{"v": c.version},
		}
		files = append(files, &pbgo.File{Path: c.path(), Raw: raw})
		clientConfigs = append(clientConfigs, c.path())
	}

	signed := map[string]any{
		"_type":        "targets",
		"spec_version": "1.0",
		"version":      targetsVersion,
		"expires":      "2032-10-24T15:10:45.097315-04:00",
		"targets":      targetEntries,
		"custom":       map[string]any{"opaque_backend_state": "eyJmb28iOiAiYmFyIn0="},
	}
	signedBytes, _ := json.Marshal(signed)
	envelope, _ := json.Marshal(map[string]any{"signed": json.RawMessage(signedBytes), "signatures": []any{}})

	return &pbgo.ClientGetConfigsResponse{
		Targets:       envelope,
		TargetFiles:   files,
		ClientConfigs: clientConfigs,
		ConfigStatus:  pbgo.ConfigStatus_CONFIG_STATUS_OK,
	}
}

// TestClient_HungListenerDoesNotBlockOthers is the always-on regression for the
// headline guarantee of the async dispatcher: a listener that hangs forever in
// OnUpdate must NOT stall the poll loop or any other listener, and shutdown must
// stay bounded (Close would block forever, so CloseTimeout must report it).
//
// It drives the real poll loop + dispatch via a scripted fetcher: one product's
// listener wedges on its first OnUpdate, while a second product's version is
// bumped on every poll. The healthy listener must receive every bump regardless.
func TestClient_HungListenerDoesNotBlockOthers(t *testing.T) {
	const (
		hungProduct    = state.ProductContainerAutoscalingSettings
		healthyProduct = state.ProductK8SActions
		bumps          = 5
	)

	script := make([]*pbgo.ClientGetConfigsResponse, bumps)
	for i := 0; i < bumps; i++ {
		script[i] = buildResponse(uint64(i+1),
			harnessConfig{hungProduct, "a", 1, `{"v":1}`}, // never changes
			harnessConfig{healthyProduct, "b", uint64(i + 1), fmt.Sprintf(`{"v":%d}`, i+1)},
		)
	}
	fetcher := &scriptedFetcher{script: script}

	c, err := NewClient(fetcher,
		WithoutTufVerification(),
		WithAgent("test", "0.0"),
		WithPollInterval(5*time.Millisecond),
	)
	require.NoError(t, err)

	// Wedged listener: blocks forever in OnUpdate until we release it at the end.
	hang := make(chan struct{})
	var entered atomic.Bool
	c.Subscribe(hungProduct, func(_ map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		entered.Store(true)
		<-hang
	})

	// Healthy listener: must keep receiving every version bump.
	var healthyDeliveries atomic.Int32
	c.Subscribe(healthyProduct, func(configs map[string]state.RawConfig, applyState func(string, state.ApplyStatus)) {
		for p := range configs {
			healthyDeliveries.Add(1)
			applyState(p, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	})

	c.Start()

	require.Eventually(t, entered.Load, 2*time.Second, time.Millisecond,
		"wedged listener never entered OnUpdate")
	require.Eventually(t, func() bool { return healthyDeliveries.Load() >= bumps }, 2*time.Second, time.Millisecond,
		"healthy listener stopped receiving updates while another listener was hung")

	// Shutdown must not hang. Close would block forever on the wedged listener,
	// so CloseTimeout must report not-drained within the bound.
	require.False(t, c.CloseTimeout(100*time.Millisecond),
		"CloseTimeout should report not-drained while a listener is hung")

	// Release the wedged worker so nothing leaks past the test, then a full
	// Close drains cleanly (ctx is already canceled; the worker exits at once).
	close(hang)
	c.Close()
}
