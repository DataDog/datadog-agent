// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MainEndpointID is the stable endpoint identifier for the primary logs endpoint.
const MainEndpointID = "main"

// AdditionalEndpointID returns the stable endpoint identifier for additional_endpoints[index].
func AdditionalEndpointID(index int) string {
	return fmt.Sprintf("additional-%d", index)
}

// EndpointTarget describes one independently filtered HTTP endpoint target.
// Filtering and reliability are independent: both reliable (including primary)
// and unreliable endpoints may have selectors.
type EndpointTarget struct {
	ID         string
	Endpoint   config.Endpoint
	Reliable   bool
	Expression string
	Evaluator  Evaluator
}

// PolicyOptions configures an EndpointPolicy built from normal Agent HTTP endpoints.
type PolicyOptions struct {
	Endpoints    *config.Endpoints
	Selectors    map[string]string
	Evaluators   map[string]Evaluator
	Compiler     Compiler
	MaxEndpoints int
}

// CompiledEndpoint is a validated, compiled endpoint in an EndpointPolicy.
type CompiledEndpoint struct {
	ID        string
	Endpoint  config.Endpoint
	Endpoints *config.Endpoints // Single-endpoint config carrying batch/compression settings.
	Reliable  bool
	Evaluator Evaluator
}

// EndpointSelection is the resolved per-record recipient split for endpoint-policy mode.
type EndpointSelection struct {
	Reliable   []string
	Optional   []string
	Generation uint64
}

// OptionalEvalError captures a suppressed evaluation error for one optional endpoint.
// Never includes record content or credentials.
type OptionalEvalError struct {
	EndpointID string
	Err        error
}

// EndpointPolicy holds validated, compiled endpoint targets for reliable-set OR
// delivery with independent optional (unreliable) endpoints.
type EndpointPolicy struct {
	endpoints   []CompiledEndpoint
	byID        map[string]CompiledEndpoint
	reliableIdx []int
	optionalIdx []int
}

// NewEndpointPolicy builds and validates an EndpointPolicy from normal Agent *config.Endpoints.
func NewEndpointPolicy(opts PolicyOptions) (*EndpointPolicy, error) {
	if opts.Endpoints == nil {
		return nil, fmt.Errorf("%w: endpoints are required", ErrInvalidDefinition)
	}
	if !opts.Endpoints.UseHTTP || opts.Endpoints.UseProto {
		return nil, fmt.Errorf("%w: endpoint routing requires normal-mode HTTP JSON endpoints", ErrInvalidDefinition)
	}
	if opts.Endpoints.BatchWait <= 0 || opts.Endpoints.BatchMaxSize <= 0 || opts.Endpoints.BatchMaxContentSize <= 0 || opts.Endpoints.BatchMaxConcurrentSend < 0 {
		return nil, fmt.Errorf("%w: invalid batch settings", ErrInvalidDefinition)
	}
	if len(opts.Endpoints.Endpoints) == 0 {
		return nil, fmt.Errorf("%w: at least one HTTP endpoint is required", ErrInvalidDefinition)
	}
	if opts.Endpoints.Main.IsMRF {
		return nil, fmt.Errorf("%w: MRF is not supported with endpoint routing", ErrInvalidDefinition)
	}
	primary := opts.Endpoints.Endpoints[0]
	if primary.Host != opts.Endpoints.Main.Host || primary.Port != opts.Endpoints.Main.Port || primary.GetAPIKey() != opts.Endpoints.Main.GetAPIKey() {
		return nil, fmt.Errorf("%w: primary endpoint mismatch", ErrInvalidDefinition)
	}

	limit := opts.MaxEndpoints
	if limit <= 0 || limit > MaxEndpoints {
		limit = MaxEndpoints
	}
	if len(opts.Endpoints.Endpoints) > limit {
		return nil, fmt.Errorf("%w: %d endpoints exceed limit %d", ErrInvalidDefinition, len(opts.Endpoints.Endpoints), limit)
	}

	knownIDs := make(map[string]struct{}, len(opts.Endpoints.Endpoints))
	targets := make([]EndpointTarget, 0, len(opts.Endpoints.Endpoints))
	for i, ep := range opts.Endpoints.Endpoints {
		if ep.IsMRF {
			return nil, fmt.Errorf("%w: MRF is not supported with endpoint routing", ErrInvalidDefinition)
		}
		id := MainEndpointID
		if i > 0 {
			id = AdditionalEndpointID(i - 1)
		}
		knownIDs[id] = struct{}{}
		targets = append(targets, EndpointTarget{
			ID:         id,
			Endpoint:   ep,
			Reliable:   ep.IsReliable(),
			Expression: opts.Selectors[id],
			Evaluator:  opts.Evaluators[id],
		})
	}
	for id := range opts.Selectors {
		if _, ok := knownIDs[id]; !ok {
			return nil, fmt.Errorf("%w: unknown selector endpoint %q", ErrInvalidDefinition, id)
		}
	}
	for id := range opts.Evaluators {
		if _, ok := knownIDs[id]; !ok {
			return nil, fmt.Errorf("%w: unknown evaluator endpoint %q", ErrInvalidDefinition, id)
		}
	}

	return NewEndpointPolicyFromTargets(targets, opts.Endpoints, limit, opts.Compiler)
}

// NewEndpointPolicyFromTargets builds an EndpointPolicy from explicit EndpointTargets
// sharing batch settings from batchTemplate.
func NewEndpointPolicyFromTargets(targets []EndpointTarget, batchTemplate *config.Endpoints, maxEndpoints int, compiler Compiler) (*EndpointPolicy, error) {
	if batchTemplate == nil || !batchTemplate.UseHTTP || batchTemplate.UseProto ||
		batchTemplate.BatchWait <= 0 || batchTemplate.BatchMaxSize <= 0 || batchTemplate.BatchMaxContentSize <= 0 || batchTemplate.BatchMaxConcurrentSend < 0 {
		return nil, fmt.Errorf("%w: valid HTTP batch settings are required", ErrInvalidDefinition)
	}
	limit := maxEndpoints
	if limit <= 0 || limit > MaxEndpoints {
		limit = MaxEndpoints
	}
	if len(targets) == 0 || len(targets) > limit {
		return nil, fmt.Errorf("%w: %d endpoints with limit %d", ErrInvalidDefinition, len(targets), limit)
	}
	if compiler == nil {
		compiler = StubCompiler{}
	}

	policy := &EndpointPolicy{
		endpoints: make([]CompiledEndpoint, 0, len(targets)),
		byID:      make(map[string]CompiledEndpoint, len(targets)),
	}
	for idx, target := range targets {
		if target.ID == "" {
			return nil, fmt.Errorf("%w: endpoint ID cannot be empty", ErrInvalidDefinition)
		}
		if _, exists := policy.byID[target.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate endpoint %q", ErrInvalidDefinition, target.ID)
		}
		if target.Endpoint.IsMRF {
			return nil, fmt.Errorf("%w: endpoint %q uses MRF", ErrInvalidDefinition, target.ID)
		}
		if target.Reliable && !target.Endpoint.IsReliable() {
			return nil, fmt.Errorf("%w: endpoint %q marked reliable but config endpoint is unreliable", ErrInvalidDefinition, target.ID)
		}
		eval := target.Evaluator
		if eval == nil {
			var err error
			eval, err = compiler.Compile(target.Expression)
			if err != nil {
				return nil, fmt.Errorf("%w: endpoint %q selector: %w", ErrInvalidDefinition, target.ID, err)
			}
			if eval == nil {
				eval = MatchAllEvaluator{}
			}
		}
		single := config.NewEndpointsWithBatchSettings(
			target.Endpoint,
			nil,
			false,
			true,
			batchTemplate.BatchWait,
			batchTemplate.BatchMaxConcurrentSend,
			batchTemplate.BatchMaxSize,
			batchTemplate.BatchMaxContentSize,
			batchTemplate.InputChanSize,
		)
		compiled := CompiledEndpoint{
			ID:        target.ID,
			Endpoint:  target.Endpoint,
			Endpoints: single,
			Reliable:  target.Reliable,
			Evaluator: eval,
		}
		policy.endpoints = append(policy.endpoints, compiled)
		policy.byID[target.ID] = compiled
		if compiled.Reliable {
			policy.reliableIdx = append(policy.reliableIdx, idx)
		} else {
			policy.optionalIdx = append(policy.optionalIdx, idx)
		}
	}
	return policy, nil
}

// Endpoints returns all compiled endpoints in declaration order.
func (p *EndpointPolicy) Endpoints() []CompiledEndpoint {
	return append([]CompiledEndpoint(nil), p.endpoints...)
}

// Endpoint returns a compiled endpoint by ID.
func (p *EndpointPolicy) Endpoint(id string) (CompiledEndpoint, bool) {
	ep, ok := p.byID[id]
	return ep, ok
}

// Evaluate evaluates all reliable selectors first, then all optional selectors.
// Any reliable selector error fails evaluation immediately with no partial selection.
// Optional selector errors suppress only that optional copy and are returned for diagnostics.
func (p *EndpointPolicy) Evaluate(msg *message.Message) (EndpointSelection, []OptionalEvalError, error) {
	view := NewRecordView(msg)
	var reliable []string
	for _, idx := range p.reliableIdx {
		ep := p.endpoints[idx]
		matched, err := ep.Evaluator.Match(view)
		if err != nil {
			return EndpointSelection{}, nil, fmt.Errorf("reliable endpoint %q evaluation failed: %w", ep.ID, err)
		}
		if matched {
			reliable = append(reliable, ep.ID)
		}
	}
	var optional []string
	var optErrs []OptionalEvalError
	for _, idx := range p.optionalIdx {
		ep := p.endpoints[idx]
		matched, err := ep.Evaluator.Match(view)
		if err != nil {
			optErrs = append(optErrs, OptionalEvalError{EndpointID: ep.ID, Err: err})
			continue
		}
		if matched {
			optional = append(optional, ep.ID)
		}
	}
	return EndpointSelection{Reliable: reliable, Optional: optional}, optErrs, nil
}
