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
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/common"
)

type onDemandTestResponse struct {
	Tests []common.SyntheticsTestConfig `json:"tests"`
}

type onDemandPoller struct {
	httpClient      *http.Client
	endpoint        string
	apiKey          string
	hostNameService hostname.Component
	log             log.Component
	timeNowFn       func() time.Time
	TestsChan       chan SyntheticsTestCtx
	done            chan struct{}
}

func newOnDemandPoller(config *onDemandPollerConfig, hostNameService hostname.Component, logger log.Component, timeNowFn func() time.Time) *onDemandPoller {
	return &onDemandPoller{
		httpClient:      &http.Client{Transport: config.httpTransport, Timeout: 10 * time.Second},
		endpoint:        "https://intake.synthetics." + config.site + "/api/unstable/synthetics/agents/tests",
		apiKey:          config.apiKey,
		hostNameService: hostNameService,
		log:             logger,
		timeNowFn:       timeNowFn,
		TestsChan:       make(chan SyntheticsTestCtx, 100),
		done:            make(chan struct{}),
	}
}

func (p *onDemandPoller) start(ctx context.Context) {
	go p.pollLoop(ctx)
}

func (p *onDemandPoller) stop() {
	<-p.done
}

func (p *onDemandPoller) pollLoop(ctx context.Context) {
	defer close(p.done)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tests, err := p.fetchTests(ctx)
			if err != nil {
				p.log.Debugf("error fetching on-demand tests: %s", err)
				continue
			}
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

func (p *onDemandPoller) fetchTests(ctx context.Context) ([]common.SyntheticsTestConfig, error) {
	hostname, err := p.hostNameService.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting hostname: %w", err)
	}

	u, err := url.Parse(p.endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}
	q := u.Query()
	q.Set("agent_hostname", hostname)
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

	var response onDemandTestResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response.Tests, nil
}
