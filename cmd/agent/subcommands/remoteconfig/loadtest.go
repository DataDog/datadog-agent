// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// loadTestParams are the command-line arguments for 'remote-config load-test'.
type loadTestParams struct {
	numClients    int
	clientTimeout time.Duration
	pollInterval  time.Duration
	duration      time.Duration
	products      []string
}

// loadTestCommand simulates many concurrent tracer clients polling this
// agent's ClientGetConfigs, to reproduce the RC mutex-contention/backlog OOM
// mechanism (see PROJECT.md / JULY-9.md) fully locally, against a real
// CoreAgentService -- no real tracer/application code is involved, only the
// wire-level poll/timeout/retry behavior dd-trace-go exhibits under contention.
func loadTestCommand(globalParams *command.GlobalParams) *cobra.Command {
	params := &loadTestParams{}
	var productsCSV string

	cmd := &cobra.Command{
		Use:   "load-test",
		Short: "Simulate many concurrent tracer clients polling this agent's remote-config service",
		Long: `Opens --clients concurrent gRPC connections to this agent's AgentSecure
service and replays dd-trace-go's remote-config poll behavior against it: a
fixed poll interval on success, a client-side timeout, and an immediate retry
-- without cancelling the prior request -- on timeout. This is a load-testing
tool for reproducing the mutex-contention/backlog-retention OOM mechanism
locally; it does not run any real tracer/application code.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			params.products = splitCSV(productsCSV)
			return fxutil.OneShot(runLoadTest,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "OFF", false),
				}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
		Hidden: true,
	}

	cmd.Flags().IntVar(&params.numClients, "clients", 300, "number of simulated tracer clients")
	cmd.Flags().DurationVar(&params.clientTimeout, "client-timeout", 10*time.Second, "per-request timeout before a simulated client abandons and retries (matches dd-trace-go's HTTP timeout)")
	cmd.Flags().DurationVar(&params.pollInterval, "poll-interval", 5*time.Second, "steady-state poll interval after a successful response (matches dd-trace-go's default)")
	cmd.Flags().DurationVar(&params.duration, "duration", 60*time.Second, "how long to run the load test")
	cmd.Flags().StringVar(&productsCSV, "products", "APM_TRACING,ASM_FEATURES,ASM_DD,ASM_DATA,AGENT_CONFIG,AGENT_TASK,LIVE_DEBUGGING,CWS_DD", "comma-separated list of products each simulated client subscribes to")

	return cmd
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadTestStats tracks the server-side-visible backlog of the simulated
// fleet: how many ClientGetConfigs calls have been invoked but not yet
// returned, independent of whether the simulated client already gave up
// waiting on them. Mirrors orphanBacklogStats in
// pkg/config/remote/service/orphan_backlog_test.go, against a real gRPC
// connection instead of an in-process call.
type loadTestStats struct {
	inFlight    atomic.Int64
	maxInFlight atomic.Int64
	completed   atomic.Int64
	abandoned   atomic.Int64
	errored     atomic.Int64
}

func (s *loadTestStats) recordInFlightDelta(delta int64) {
	v := s.inFlight.Add(delta)
	for {
		m := s.maxInFlight.Load()
		if v <= m || s.maxInFlight.CompareAndSwap(m, v) {
			return
		}
	}
}

func runLoadTest(params *loadTestParams, conf config.Component, ipcComp ipc.Component) error {
	if !configUtils.IsRemoteConfigEnabled(conf) {
		return errors.New("remote configuration is not enabled")
	}

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}
	cmdPort := pkgconfigsetup.GetIPCPort()
	tlsConfig := ipcComp.GetTLSClientConfig()
	authToken := ipcComp.GetAuthToken()

	fmt.Printf("Starting remote-config load test: clients=%d poll-interval=%s client-timeout=%s duration=%s products=%v\n",
		params.numClients, params.pollInterval, params.clientTimeout, params.duration, params.products)

	stats := &loadTestStats{}
	runCtx, cancel := context.WithTimeout(context.Background(), params.duration)
	defer cancel()

	// outer tracks the per-client poll loops; inner tracks the individual
	// (possibly orphaned) ClientGetConfigs calls, so we can wait for the
	// backlog to fully drain after the active phase ends.
	var outer, inner sync.WaitGroup
	for i := 0; i < params.numClients; i++ {
		idx := i
		outer.Add(1)
		go func() {
			defer outer.Done()
			runSimulatedClient(runCtx, idx, ipcAddress, cmdPort, tlsConfig, authToken, params.products, params.clientTimeout, params.pollInterval, stats, &inner)
		}()
	}

	start := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
loop:
	for {
		select {
		case <-runCtx.Done():
			break loop
		case <-ticker.C:
			fmt.Printf("t=%-6s inFlight=%d maxInFlight=%d completed=%d abandoned=%d errored=%d\n",
				time.Since(start).Round(time.Second), stats.inFlight.Load(), stats.maxInFlight.Load(),
				stats.completed.Load(), stats.abandoned.Load(), stats.errored.Load())
		}
	}

	outer.Wait()
	fmt.Println("Active phase done, waiting for backlog to drain...")
	drained := make(chan struct{})
	go func() {
		inner.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(2 * time.Minute):
		fmt.Println("WARNING: backlog did not fully drain within 2m; reporting partial results")
	}

	fmt.Printf("Final: completed=%d abandoned=%d errored=%d maxInFlight=%d finalInFlight=%d\n",
		stats.completed.Load(), stats.abandoned.Load(), stats.errored.Load(), stats.maxInFlight.Load(), stats.inFlight.Load())

	return nil
}

// runSimulatedClient mimics dd-trace-go's remote-config poller under
// contention: fire a request, and if it doesn't complete within
// clientTimeout, stop waiting on it and immediately fire a new one instead
// of arming the next poll tick. The original call is not cancelled in a way
// that stops the underlying request against the real agent -- it keeps
// running against the real ClientGetConfigs call, exactly like the orphaned
// goroutines described in JULY-9.md.
func runSimulatedClient(
	ctx context.Context,
	idx int,
	ipcAddress, cmdPort string,
	tlsConfig *tls.Config,
	authToken string,
	products []string,
	clientTimeout, pollInterval time.Duration,
	stats *loadTestStats,
	inner *sync.WaitGroup,
) {
	clientID := fmt.Sprintf("rc-loadtest-%d-%s", idx, randHex(6))
	baseCtx := metadata.NewOutgoingContext(context.Background(), metadata.MD{
		"authorization": []string{"Bearer " + authToken},
	})

	// Each simulated client gets its own connection, matching real tracers
	// (separate processes) rather than multiplexing many clients over one
	// HTTP/2 connection's MaxConcurrentStreams budget.
	cli, err := agentgrpc.GetDDAgentSecureClient(baseCtx, ipcAddress, cmdPort, tlsConfig)
	if err != nil {
		stats.errored.Add(1)
		return
	}

	req := &pbgo.ClientGetConfigsRequest{
		Client: &pbgo.Client{
			Id:       clientID,
			IsTracer: true,
			ClientTracer: &pbgo.ClientTracer{
				RuntimeId:     clientID + "-runtime",
				Language:      "loadtest",
				TracerVersion: "0.0.0",
				Service:       "rc-loadtest",
			},
			Products: products,
			// TargetsVersion 0 means every poll looks like it needs a full
			// update, matching a worst-case/never-caching client. See
			// PHASE2_GUARDRAILS.md and PROJECT.md for why this maximizes
			// the per-request critical-section cost we're trying to trigger.
			State: &pbgo.ClientState{RootVersion: 1, TargetsVersion: 0},
		},
	}

	for ctx.Err() == nil {
		reqCtx, cancel := context.WithTimeout(baseCtx, clientTimeout)

		inner.Add(1)
		stats.recordInFlightDelta(1)
		done := make(chan error, 1)
		go func() {
			defer inner.Done()
			defer stats.recordInFlightDelta(-1)
			_, callErr := cli.ClientGetConfigs(reqCtx, req)
			done <- callErr
		}()

		abandoned := false
		select {
		case callErr := <-done:
			if callErr != nil {
				stats.errored.Add(1)
			} else {
				stats.completed.Add(1)
			}
		case <-reqCtx.Done():
			// The simulated tracer gives up here, exactly like dd-trace-go's
			// HTTP timeout. The goroutine above keeps running against the
			// real agent regardless.
			stats.abandoned.Add(1)
			abandoned = true
		case <-ctx.Done():
			cancel()
			return
		}
		cancel()

		if abandoned {
			// dd-trace-go retries immediately on timeout rather than
			// waiting for the next poll tick (PHASE2_GUARDRAILS.md).
			continue
		}
		select {
		case <-time.After(pollInterval):
		case <-ctx.Done():
			return
		}
	}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
