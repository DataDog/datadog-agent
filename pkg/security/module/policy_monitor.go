// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// policyMetricRate defines how often the policy metric will be sent
	policyMetricRate = 30 * time.Second
)

// Policy describes policy related information
type Policy struct {
	Name    string
	Source  string
	Version string
}

// PolicyMonitor defines a policy monitor
type PolicyMonitor struct {
	sync.RWMutex

	statsdClient statsd.ClientInterface
	policies     map[string]Policy
}

// AddPolicies add policies to the monitor
func (p *PolicyMonitor) AddPolicies(policies []*rules.Policy) {
	p.Lock()
	defer p.Unlock()

	for _, policy := range policies {
		p.policies[policy.Name] = Policy{Name: policy.Name, Source: policy.Source, Version: policy.Version}
	}
}

// Start the monitor
func (p *PolicyMonitor) Start(ctx context.Context) {
	go func() {
		timer := time.NewTicker(policyMetricRate)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-timer.C:
				p.RLock()
				for _, policy := range p.policies {
					tags := []string{
						"policy_name:" + policy.Name,
						"policy_source:" + policy.Source,
						"policy_version:" + policy.Version,
					}

					if err := p.statsdClient.Gauge(metrics.MetricPolicy, 1, tags, 1.0); err != nil {
						log.Error(fmt.Errorf("failed to send policy metric: %w", err))
					}
				}
				p.RUnlock()
			}
		}
	}()
}

// NewPolicyMonitor returns a new Policy monitor
func NewPolicyMonitor(statsdClient statsd.ClientInterface) *PolicyMonitor {
	return &PolicyMonitor{
		statsdClient: statsdClient,
		policies:     make(map[string]Policy),
	}
}
