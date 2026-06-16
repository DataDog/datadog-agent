// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package client

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file is a MANUAL, LOCAL harness for eyeballing the RC-ASYNC behaviour
// without a real remote-config backend or a staging deploy. It reuses the
// scripted fetcher + payload builders from rcasync_scripted_test.go and adds
// console logging so you can watch — via the RC-ASYNC log lines — what happens
// when a listener hangs while newer versions of another product arrive.
//
// It is gated behind an env var so it never runs in CI (it sleeps for several
// seconds on purpose). Run it locally with:
//
//	RC_ASYNC_HARNESS=1 dda inv test --targets=./pkg/config/remote/client -v
//
// or, equivalently, with the raw toolchain (the `test` build tag is required
// for the console-logger helper):
//
//	RC_ASYNC_HARNESS=1 go test -tags test -v -run TestRCAsyncLocalHarness ./pkg/config/remote/client/
//
// The automated, always-on version of this guarantee lives in
// rcasync_scripted_test.go (TestClient_HungListenerDoesNotBlockOthers). This
// harness is throwaway scaffolding — delete it along with the RC-ASYNC log
// lines once the behaviour is confirmed.

func TestRCAsyncLocalHarness(t *testing.T) {
	if os.Getenv("RC_ASYNC_HARNESS") == "" {
		t.Skip("manual harness; set RC_ASYNC_HARNESS=1 to run")
	}

	// Send RC-ASYNC (and all other) logs to stdout at info so they're visible.
	logger, err := log.LoggerFromWriterWithMinLevel(os.Stdout, log.InfoLvl)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	log.SetupLogger(logger, "info")

	const (
		autoscaling = state.ProductContainerAutoscalingSettings
		k8sactions  = state.ProductK8SActions
	)

	// The script: one poll per k8s-actions version bump. Each poll re-sends the
	// FULL active set (as real RC does), so nothing looks "removed". Autoscaling
	// stays at v1 the whole time; k8s-actions bumps v1..v6 across the polls.
	const k8sBumps = 6
	script := make([]*pbgo.ClientGetConfigsResponse, k8sBumps)
	for i := 0; i < k8sBumps; i++ {
		script[i] = buildResponse(uint64(i+1),
			harnessConfig{autoscaling, "autoscaler-1", 1, `{"name":"autoscaler-1","v":1}`},
			harnessConfig{k8sactions, "action-1", uint64(i + 1), fmt.Sprintf(`{"action":"scale","v":%d}`, i+1)},
		)
	}
	fetcher := &scriptedFetcher{script: script}

	c, err := NewClient(fetcher,
		WithoutTufVerification(),
		WithAgent("harness", "0.0"),
		WithPollInterval(500*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// HUNG listener: container autoscaling. Its OnUpdate blocks indefinitely on
	// `hang` — simulating a listener that never returns (deadlock, stuck syscall,
	// waiting on a torn-down dependency, etc.). The whole point of the async
	// dispatcher is that this must NOT stall the poll loop or any other listener.
	hang := make(chan struct{})
	c.Subscribe(autoscaling, func(configs map[string]state.RawConfig, applyState func(string, state.ApplyStatus)) {
		log.Infof("RC-ASYNC [harness] >>> AUTOSCALING listener ENTERED OnUpdate (%d configs) and is now HANGING FOREVER", len(configs))
		<-hang // never closed during the observation window
		log.Infof("RC-ASYNC [harness] <<< AUTOSCALING listener released")
		for p := range configs {
			applyState(p, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	})

	// Healthy listener: kubernetes actions. Acks immediately. We expect it to
	// keep receiving every k8s-actions version while autoscaling is wedged.
	var k8sDeliveries atomic.Int32
	c.Subscribe(k8sactions, func(configs map[string]state.RawConfig, applyState func(string, state.ApplyStatus)) {
		for p, cfg := range configs {
			k8sDeliveries.Add(1)
			log.Infof("RC-ASYNC [harness] K8S-ACTIONS listener got %s @v%d — acking immediately", p, cfg.Metadata.Version)
			applyState(p, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	})

	log.Infof("RC-ASYNC [harness] ===== starting client; autoscaling will hang, k8s-actions should keep flowing =====")
	c.Start()

	// Observation window: long enough for all k8s-actions bumps to be delivered
	// while autoscaling is stuck in its very first OnUpdate.
	time.Sleep(5 * time.Second)

	got := k8sDeliveries.Load()
	log.Infof("RC-ASYNC [harness] ===== observed %d k8s-actions deliveries while autoscaling was hung (expected %d) =====", got, k8sBumps)
	if int(got) < k8sBumps {
		t.Errorf("expected k8s-actions to keep flowing while autoscaling hung: got %d deliveries, want >= %d", got, k8sBumps)
	}

	// Shutdown with a STUCK listener: Close would block forever, so use
	// CloseTimeout. It must return false (autoscaling is still wedged) and leak
	// only that one worker — the poll loop and the healthy worker still tear down.
	log.Infof("RC-ASYNC [harness] ===== CloseTimeout(2s) with autoscaling still hung =====")
	drained := c.CloseTimeout(2 * time.Second)
	log.Infof("RC-ASYNC [harness] CloseTimeout returned drained=%t (expected false — a listener is stuck)", drained)
	if drained {
		t.Error("expected CloseTimeout to report NOT drained while a listener is hung")
	}

	// Release the wedged listener so the leaked goroutine can exit cleanly now
	// that the observation is done (good hygiene; not part of the assertion).
	close(hang)
	time.Sleep(200 * time.Millisecond)
	log.Infof("RC-ASYNC [harness] ===== done =====")
}
