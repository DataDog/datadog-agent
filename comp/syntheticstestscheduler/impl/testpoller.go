// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
)

const (
	pollingFrequency   = 2 * time.Second
	httpRequestTimeout = 10 * time.Second
	// maxConsecutiveErrors is the number of consecutive poll failures after
	// which the poller is considered unhealthy and the in-memory fallback
	// scheduler takes over.
	maxConsecutiveErrors = 5
)

type testPollerResponse struct {
	Tests []common.SyntheticsTestConfig `json:"tests"`
}

type testPoller struct {
	httpClient      *http.Client
	endpoint        string
	apiKey          string
	agentVersion    string
	hostNameService hostname.Component
	log             log.Component
	timeNowFn       func() time.Time
	TestsChan       chan SyntheticsTestCtx
	done            chan struct{}

	mu                sync.Mutex
	consecutiveErrors int
	healthy           bool
}

func newTestPoller(config *testPollerConfig, hostNameService hostname.Component, logger log.Component, timeNowFn func() time.Time) *testPoller {
	return &testPoller{
		httpClient:      &http.Client{Transport: config.httpTransport, Timeout: httpRequestTimeout},
		endpoint:        "https://intake.synthetics." + config.site + "/api/unstable/synthetics/agents/tests",
		apiKey:          config.apiKey,
		agentVersion:    config.agentVersion,
		hostNameService: hostNameService,
		log:             logger,
		timeNowFn:       timeNowFn,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
		healthy:         true,
	}
}

func (p *testPoller) start(ctx context.Context) {
	go p.pollLoop(ctx)
}

func (p *testPoller) stop() {
	<-p.done
}

// isHealthy reports whether the poller is currently considered healthy.
// When it returns false, the scheduler's in-memory flush loop should take over.
func (p *testPoller) isHealthy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.healthy
}

func (p *testPoller) markSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveErrors = 0
	p.healthy = true
}

func (p *testPoller) markFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveErrors++
	if p.consecutiveErrors >= maxConsecutiveErrors {
		p.healthy = false
	}
}

func (p *testPoller) pollLoop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(pollingFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tests, err := p.fetchTests(ctx)
			if err != nil {
				p.markFailure()
				p.log.Debugf("error fetching tests: %s", err)
				continue
			}
			p.markSuccess()
			for _, test := range tests {
				select {
				case p.TestsChan <- SyntheticsTestCtx{
					nextRun: p.timeNowFn(),
					cfg:     test,
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (p *testPoller) fetchTests(ctx context.Context) ([]common.SyntheticsTestConfig, error) {
	hostname, err := p.hostNameService.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %w", err)
	}

	u, err := url.Parse(p.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}
	q := url.Values{}
	q.Set("agent_hostname", hostname)
	q.Set("agent_version", p.agentVersion)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("DD-API-KEY", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response testPollerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Tests, nil
}
