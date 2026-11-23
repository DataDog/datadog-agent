// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// MaxFilterEndpoints caps the number of configured endpoints supported by
// inline uint64 bitmask endpoint filtering without heap allocations.
const MaxFilterEndpoints = 64

var (
	// ErrSelectorCompilerNotConfigured is returned by StubSelectorCompiler
	// until the OTTL filtering integration (PR #59688) lands.
	ErrSelectorCompilerNotConfigured = errors.New("endpoint selector compiler is not configured (OTTL integration stubbed)")
	// ErrInvalidEndpointFilterPolicy indicates invalid endpoint filtering configuration.
	ErrInvalidEndpointFilterPolicy = errors.New("invalid endpoint filter policy")
)

// LogView exposes a read-only view of a log message after common processing
// and metadata preparation, before rendering and JSON encoding.
//
// Selectors inspect local unfiltered tags (Tags), origin metadata (Service,
// Source), content, status, and structured JSON fields (StructuredAttribute)
// without mutating shared message state.
type LogView struct {
	msg *message.Message
}

// NewLogView wraps a processed message in a read-only view for endpoint selection.
func NewLogView(msg *message.Message) LogView {
	return LogView{msg: msg}
}

// Content returns the message content bytes after common processing rules.
func (v LogView) Content() []byte {
	if v.msg == nil {
		return nil
	}
	return v.msg.GetContent()
}

// Status returns the message severity/status (for example "info", "notice", "error").
func (v LogView) Status() string {
	if v.msg == nil {
		return ""
	}
	return v.msg.GetStatus()
}

// Hostname returns the message hostname if already populated on the message.
func (v LogView) Hostname() string {
	if v.msg == nil {
		return ""
	}
	return v.msg.Hostname
}

// Service returns the message origin service, or "" if unavailable.
func (v LogView) Service() string {
	if v.msg == nil || v.msg.Origin == nil {
		return ""
	}
	return v.msg.Origin.Service()
}

// Source returns the message origin source (including remap_source), or "" if unavailable.
func (v LogView) Source() string {
	if v.msg == nil || v.msg.Origin == nil {
		return ""
	}
	return v.msg.Origin.Source()
}

// Tags returns the local unfiltered Agent tags from the message origin.
// Outgoing tag_filters are applied later during transport encoding, so local
// tags remain matchable even when export filtering removes them.
func (v LogView) Tags() []string {
	if v.msg == nil || v.msg.Origin == nil {
		return nil
	}
	return v.msg.Origin.Tags()
}

// StructuredAttribute looks up a structured JSON attribute by key/path while
// structured fields remain accessible prior to rendering.
func (v LogView) StructuredAttribute(key string) (any, bool) {
	if v.msg == nil {
		return nil, false
	}
	return v.msg.GetStructuredAttribute(key)
}

// RawMessage returns the underlying message for read-only adapters (such as a
// future OTTL context adapter). Callers must not mutate the returned message.
func (v LogView) RawMessage() *message.Message {
	return v.msg
}

// EndpointSelector evaluates whether a processed log record should be delivered
// to a specific endpoint. Implementations must be safe for concurrent calls,
// local, bounded, and must not mutate the message.
type EndpointSelector interface {
	Match(view LogView) (bool, error)
}

// SelectorFunc adapts a plain function to EndpointSelector.
type SelectorFunc func(view LogView) (bool, error)

// Match invokes f(view).
func (f SelectorFunc) Match(view LogView) (bool, error) {
	return f(view)
}

// SelectorCompiler compiles a raw endpoint filter expression into an
// EndpointSelector. OTTL integration will supply the production implementation
// once the OTTL filtering PR lands.
type SelectorCompiler interface {
	Compile(expr string) (EndpointSelector, error)
}

// StubSelectorCompiler is the placeholder compiler used until OTTL filtering
// lands. Compiling an empty expression returns nil (unfiltered); any non-empty
// expression returns ErrSelectorCompilerNotConfigured.
type StubSelectorCompiler struct{}

// Compile returns nil for an empty expression and ErrSelectorCompilerNotConfigured
// for any non-empty expression.
func (StubSelectorCompiler) Compile(expr string) (EndpointSelector, error) {
	if expr == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("%w: cannot compile expression %q", ErrSelectorCompilerNotConfigured, expr)
}

// EndpointEvalError records a per-endpoint selector evaluation failure.
type EndpointEvalError struct {
	EndpointIndex int
	EndpointHost  string
	Reliable      bool
	Err           error
}

// Error formats the evaluation error with endpoint metadata.
func (e EndpointEvalError) Error() string {
	kind := "optional"
	if e.Reliable {
		kind = "reliable"
	}
	return fmt.Sprintf("endpoint %d (%s, %s) selector evaluation failed: %v", e.EndpointIndex, e.EndpointHost, kind, e.Err)
}

type evaluatedEndpoint struct {
	index    int
	bit      uint64
	host     string
	reliable bool
	isMRF    bool
	selector EndpointSelector
}

// EndpointFilterPolicy holds validated endpoints and precomputed masks for
// inline bitmask endpoint selection (zero heap allocations for mask
// computation and non-allocating selectors on the error-free path).
type EndpointFilterPolicy struct {
	endpoints          []config.Endpoint
	ids                []string
	alwaysReliableMask uint64
	alwaysOptionalMask uint64
	unfilteredMRFMask  uint64
	allReliableMask    uint64
	allOptionalMask    uint64
	allMRFMask         uint64
	evaluated          []evaluatedEndpoint
	hasSelectors       bool
}

// NewEndpointFilterPolicy validates normal-mode HTTP endpoints and their
// optional per-endpoint selectors. Nil entries in selectors mean unfiltered
// (always match, subject to MRF gating for MRF endpoints).
func NewEndpointFilterPolicy(endpoints *config.Endpoints, selectors []EndpointSelector) (*EndpointFilterPolicy, error) {
	if endpoints == nil || !endpoints.UseHTTP || endpoints.UseProto || len(endpoints.Endpoints) == 0 {
		return nil, fmt.Errorf("%w: requires HTTP JSON endpoints", ErrInvalidEndpointFilterPolicy)
	}
	if endpoints.BatchWait <= 0 || endpoints.BatchMaxSize <= 0 || endpoints.BatchMaxContentSize <= 0 || endpoints.BatchMaxConcurrentSend < 0 {
		return nil, fmt.Errorf("%w: invalid batch settings", ErrInvalidEndpointFilterPolicy)
	}
	if len(endpoints.Endpoints) > MaxFilterEndpoints {
		return nil, fmt.Errorf("%w: %d endpoints exceeds limit %d", ErrInvalidEndpointFilterPolicy, len(endpoints.Endpoints), MaxFilterEndpoints)
	}
	if len(selectors) > len(endpoints.Endpoints) {
		return nil, fmt.Errorf("%w: %d selectors for %d endpoints", ErrInvalidEndpointFilterPolicy, len(selectors), len(endpoints.Endpoints))
	}

	p := &EndpointFilterPolicy{
		endpoints: append([]config.Endpoint(nil), endpoints.Endpoints...),
		ids:       make([]string, len(endpoints.Endpoints)),
	}
	for i, ep := range p.endpoints {
		p.ids[i] = "ep-" + strconv.Itoa(i)
		bit := uint64(1) << uint(i)
		var sel EndpointSelector
		if i < len(selectors) {
			sel = selectors[i]
		}
		if ep.IsReliable() {
			p.allReliableMask |= bit
		} else {
			p.allOptionalMask |= bit
		}
		if ep.IsMRF {
			p.allMRFMask |= bit
		}
		if sel != nil {
			p.hasSelectors = true
			p.evaluated = append(p.evaluated, evaluatedEndpoint{
				index:    i,
				bit:      bit,
				host:     ep.Host,
				reliable: ep.IsReliable(),
				isMRF:    ep.IsMRF,
				selector: sel,
			})
			continue
		}
		if ep.IsMRF {
			p.unfilteredMRFMask |= bit
		} else if ep.IsReliable() {
			p.alwaysReliableMask |= bit
		} else {
			p.alwaysOptionalMask |= bit
		}
	}
	return p, nil
}

// HasSelectors reports whether at least one endpoint has a non-nil selector.
func (p *EndpointFilterPolicy) HasSelectors() bool {
	return p.hasSelectors
}

// EndpointCount returns the total number of configured endpoints.
func (p *EndpointFilterPolicy) EndpointCount() int {
	return len(p.endpoints)
}

// Endpoint returns the validated endpoint at index.
func (p *EndpointFilterPolicy) Endpoint(index int) config.Endpoint {
	return p.endpoints[index]
}

// EndpointID returns the stable progress group identifier for endpoint index.
func (p *EndpointFilterPolicy) EndpointID(index int) string {
	return p.ids[index]
}

// Evaluate computes the matching reliable and optional endpoint bitmasks for msg.
//
// Error semantics:
//   - Reliable endpoint evaluation error: fail-open (include endpoint) to
//     preserve the zero-loss guarantee and align with common filter behavior.
//   - Optional (unreliable) endpoint evaluation error: fail-closed (exclude
//     endpoint) to prevent unintended fanout or queue saturation.
//
// MRF endpoints are only eligible when msg.IsMRFAllow is true.
func (p *EndpointFilterPolicy) Evaluate(msg *message.Message) (reliableMask, optionalMask uint64, evalErrs []EndpointEvalError) {
	reliableMask = p.alwaysReliableMask
	optionalMask = p.alwaysOptionalMask

	allowMRF := msg != nil && msg.IsMRFAllow
	if allowMRF && p.unfilteredMRFMask != 0 {
		reliableMask |= p.unfilteredMRFMask & p.allReliableMask
		optionalMask |= p.unfilteredMRFMask & p.allOptionalMask
	}

	if len(p.evaluated) == 0 {
		return reliableMask, optionalMask, nil
	}

	view := NewLogView(msg)
	for _, ep := range p.evaluated {
		if ep.isMRF && !allowMRF {
			continue
		}
		matched, err := ep.selector.Match(view)
		if err != nil {
			evalErrs = append(evalErrs, EndpointEvalError{
				EndpointIndex: ep.index,
				EndpointHost:  ep.host,
				Reliable:      ep.reliable,
				Err:           err,
			})
			if ep.reliable {
				// Fail-open for reliable endpoints.
				reliableMask |= ep.bit
			}
			continue
		}
		if matched {
			if ep.reliable {
				reliableMask |= ep.bit
			} else {
				optionalMask |= ep.bit
			}
		}
	}
	return reliableMask, optionalMask, evalErrs
}
