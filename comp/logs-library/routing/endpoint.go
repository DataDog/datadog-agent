// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// EndpointTarget defines one independently filtered HTTP JSON endpoint target.
// Filtering and reliability are independent: both reliable (including primary)
// and unreliable endpoints may supply an Evaluator. A nil Evaluator matches
// every surviving record.
type EndpointTarget struct {
	ID        string
	Endpoints *config.Endpoints
	Evaluator Evaluator
}

// EndpointDefinitions validates an injected set of EndpointTargets before any
// workers start and evaluates per-record endpoint membership.
type EndpointDefinitions struct {
	targets map[string]EndpointTarget
	order   []string
}

// NewEndpointDefinitions validates injected endpoint targets.
// Every target must be a single HTTP JSON non-MRF endpoint with positive batch
// settings, and at least one reliable target must be present.
func NewEndpointDefinitions(targets []EndpointTarget) (*EndpointDefinitions, error) {
	if len(targets) == 0 || len(targets) > MaxEndpoints {
		return nil, fmt.Errorf("%w: %d endpoints with limit %d", ErrInvalidDefinition, len(targets), MaxEndpoints)
	}
	d := &EndpointDefinitions{
		targets: make(map[string]EndpointTarget, len(targets)),
		order:   make([]string, 0, len(targets)),
	}
	reliableCount := 0
	for _, target := range targets {
		if target.ID == "" || target.Endpoints == nil || !target.Endpoints.UseHTTP || target.Endpoints.UseProto ||
			len(target.Endpoints.Endpoints) != 1 ||
			target.Endpoints.BatchWait <= 0 || target.Endpoints.BatchMaxSize <= 0 ||
			target.Endpoints.BatchMaxContentSize <= 0 || target.Endpoints.BatchMaxConcurrentSend < 0 {
			return nil, fmt.Errorf("%w: endpoint %q needs a single HTTP JSON destination with valid batch settings", ErrInvalidDefinition, target.ID)
		}
		ep := target.Endpoints.Endpoints[0]
		if ep.Host != target.Endpoints.Main.Host || ep.Port != target.Endpoints.Main.Port ||
			ep.GetAPIKey() != target.Endpoints.Main.GetAPIKey() || ep.IsReliable() != target.Endpoints.Main.IsReliable() {
			return nil, fmt.Errorf("%w: endpoint %q main destination mismatch", ErrInvalidDefinition, target.ID)
		}
		if target.Endpoints.Main.IsMRF || ep.IsMRF {
			return nil, fmt.Errorf("%w: endpoint %q uses MRF", ErrInvalidDefinition, target.ID)
		}
		if _, exists := d.targets[target.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate endpoint %q", ErrInvalidDefinition, target.ID)
		}
		if target.Evaluator == nil {
			target.Evaluator = MatchAllEvaluator{}
		}
		if target.Endpoints.Main.IsReliable() {
			reliableCount++
		}
		d.targets[target.ID] = target
		d.order = append(d.order, target.ID)
	}
	if reliableCount == 0 {
		return nil, fmt.Errorf("%w: at least one reliable endpoint is required", ErrInvalidDefinition)
	}
	return d, nil
}

// Targets returns all validated endpoint targets in declaration order.
func (d *EndpointDefinitions) Targets() []EndpointTarget {
	result := make([]EndpointTarget, 0, len(d.order))
	for _, id := range d.order {
		result = append(result, d.targets[id])
	}
	return result
}

// Evaluate runs every target's selector against a read-only RecordView and
// splits matching endpoint IDs into reliable and optional lists.
// Selector errors do not cancel matches on other endpoints.
func (d *EndpointDefinitions) Evaluate(view RecordView) (reliable, optional []string, reliableError bool, err error) {
	for _, id := range d.order {
		target := d.targets[id]
		matched, matchErr := target.Evaluator.Match(view)
		if matchErr != nil {
			// An evaluator error can contain log content. Report only the endpoint ID.
			err = errors.Join(err, fmt.Errorf("%w: endpoint %q selector failed", ErrInvalidSelection, id))
			reliableError = reliableError || target.Endpoints.Main.IsReliable()
			continue
		}
		if !matched {
			continue
		}
		if target.Endpoints.Main.IsReliable() {
			reliable = append(reliable, id)
		} else {
			optional = append(optional, id)
		}
	}
	return reliable, optional, reliableError, err
}
