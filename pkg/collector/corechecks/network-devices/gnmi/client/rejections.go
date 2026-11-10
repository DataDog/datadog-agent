// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Initial publication can be slow on real targets; do not confuse a short
	// debug timeout with a request rejection. The overall pass is still bounded.
	isolationTimeout       = 2 * time.Minute
	probeTimeout           = 30 * time.Second
	rejectionRetryInterval = 5 * time.Minute
)

type rejectedSubscription struct {
	path       string
	err        string
	retryAfter time.Time
}

// Only explicit request/schema failures trigger isolation. Authentication,
// transport, cancellation and resource-limit failures must not disable paths.
// Even these codes do not prove a path is unsupported: mode/encoding failures
// can use the same codes. Isolation requires an accepted control subscription.
func isRequestRejection(err error) bool {
	switch status.Code(err) {
	case codes.InvalidArgument, codes.NotFound, codes.Unimplemented:
		return true
	default:
		return false
	}
}

// subscriptionResponseError preserves deprecated in-band gNMI errors as gRPC
// statuses, so they follow exactly the same handling as Recv() status errors.
func subscriptionResponseError(response *gnmipb.SubscribeResponse) error {
	payload, ok := response.GetResponse().(*gnmipb.SubscribeResponse_Error)
	if !ok {
		return nil
	}
	if payload == nil || payload.Error == nil { //nolint:staticcheck // SA1019: Supported targets still send deprecated in-band errors.
		return status.Error(codes.Unknown, "empty gNMI subscription error response")
	}
	code := codes.Code(payload.Error.GetCode()) //nolint:staticcheck // SA1019: Preserve the legacy device error code.
	if code == codes.OK || code > codes.Unauthenticated {
		code = codes.Unknown
	}
	return status.Error(code, payload.Error.GetMessage()) //nolint:staticcheck // SA1019: Preserve the legacy device error message.
}

// Rejection identity follows the actual request encoder, not diagnostic labels
// or cache aliases. Only selectors producing the same wire path share rejection
// decisions, including when the encoder gains support for additional selectors.
func subscriptionID(spec SubscriptionSpec) string {
	path, err := subscribePathFromSpec(spec)
	if err != nil {
		return "invalid=" + strconv.Quote(spec.Path) + "{" + formatQuotedKeys(spec.Keys) + "}"
	}
	return subscriptionWireID(path)
}

// Keep this separate from cache identity: targets may reject one spelling while
// accepting another that the reporting cache treats as equivalent.
func subscriptionWireID(path *gnmipb.Path) string {
	var builder strings.Builder
	builder.WriteString("origin=")
	builder.WriteString(strconv.Quote(path.GetOrigin()))
	builder.WriteString(",target=")
	builder.WriteString(strconv.Quote(path.GetTarget()))
	for _, elem := range path.GetElem() {
		builder.WriteByte('/')
		builder.WriteString(strconv.Quote(elem.GetName()))
		builder.WriteByte('{')
		builder.WriteString(formatQuotedKeys(elem.GetKey()))
		builder.WriteByte('}')
	}
	return builder.String()
}

type isolationResult struct {
	rejected map[string]error
	accepted bool
}

// isolateSubscriptions bisects a known-rejected batch. It opens at most one
// diagnostic stream at a time and at most 2*N probes, additionally bounded by
// ctx. Silence/deadline expiry is inconclusive, never an unsupported-path flag.
// A fatal connection error discards the result. If no control group is accepted,
// the caller must retain the original batch (e.g. an unsupported encoding).
func isolateSubscriptions(ctx context.Context, specs []SubscriptionSpec, probe func(context.Context, []SubscriptionSpec) error) (isolationResult, error) {
	result := isolationResult{rejected: make(map[string]error)}
	if len(specs) < 2 {
		return result, nil
	}
	middle := len(specs) / 2
	pending := [][]SubscriptionSpec{specs[:middle], specs[middle:]}
	for attempts := 0; len(pending) > 0 && attempts < 2*len(specs); attempts++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		group := pending[0]
		pending = pending[1:]
		err := probe(ctx, group)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err == nil {
			result.accepted = true
			continue
		}
		if isRequestRejection(err) {
			if len(group) == 1 {
				result.rejected[subscriptionID(group[0])] = err
			} else {
				middle := len(group) / 2
				pending = append(pending, group[:middle], group[middle:])
			}
			continue
		}
		if status.Code(err) == codes.DeadlineExceeded {
			continue // A silent publisher says nothing about path support.
		}
		return isolationResult{}, err
	}
	return result, nil
}

func (c *Client) probeSubscriptions(ctx context.Context, conn *grpc.ClientConn, specs []SubscriptionSpec) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel() // Always closes the diagnostic RPC, including after sync/error.
	stream, err := gnmipb.NewGNMIClient(conn).Subscribe(ctx)
	if err != nil {
		return err
	}
	request, err := c.buildSubscribeRequestForSpecs(specs)
	if err != nil {
		return err
	}
	if err := stream.Send(request); err != nil {
		return err
	}
	for {
		response, err := stream.Recv()
		if err != nil {
			return err
		}
		if err := subscriptionResponseError(response); err != nil {
			return err
		}
		if payload, ok := response.GetResponse().(*gnmipb.SubscribeResponse_SyncResponse); ok {
			if payload == nil || !payload.SyncResponse {
				return status.Error(codes.Unknown, "received false sync response during path isolation")
			}
			return nil // Request accepted, not proof of populated metric support.
		}
		// Probe updates are deliberately not published or merged into the cache.
	}
}

func (c *Client) activeSubscriptions() []SubscriptionSpec {
	specs := buildSubscriptionSpecs(c.cfg)
	now := c.opt.clock.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	active := make([]SubscriptionSpec, 0, len(specs))
	for _, spec := range specs {
		key := subscriptionID(spec)
		if rejected, ok := c.rejectedSubscriptions[key]; ok {
			if now.Before(rejected.retryAfter) {
				continue
			}
			delete(c.rejectedSubscriptions, key)
		}
		active = append(active, spec)
	}
	return active
}

func (c *Client) isolateRequest(ctx context.Context, conn *grpc.ClientConn, specs []SubscriptionSpec) (bool, error) {
	discoveryCtx, cancel := context.WithTimeout(ctx, isolationTimeout)
	defer cancel()
	result, err := isolateSubscriptions(discoveryCtx, specs, func(ctx context.Context, group []SubscriptionSpec) error {
		return c.probeSubscriptions(ctx, conn, group)
	})
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	// A bounded discovery deadline may leave some paths unknown. Keep those
	// paths enabled; only confirmed singleton failures can be excluded below.
	if err != nil && discoveryCtx.Err() != context.DeadlineExceeded {
		return false, err
	}
	if !result.accepted || len(result.rejected) == 0 {
		return false, nil
	}
	labels := make(map[string]string, len(specs))
	for _, spec := range specs {
		labels[subscriptionID(spec)] = spec.String()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rejectedSubscriptions == nil {
		c.rejectedSubscriptions = make(map[string]rejectedSubscription)
	}
	retryAfter := c.opt.clock.Now().Add(rejectionRetryInterval)
	for path, err := range result.rejected {
		c.rejectedSubscriptions[path] = rejectedSubscription{path: labels[path], err: err.Error(), retryAfter: retryAfter}
	}
	return true, nil
}

// Called with c.mu held. Return a copy so status consumers cannot mutate state.
func (c *Client) rejectedSubscriptionErrors() map[string]string {
	if len(c.rejectedSubscriptions) == 0 {
		return nil
	}
	errors := make(map[string]string, len(c.rejectedSubscriptions))
	for _, rejected := range c.rejectedSubscriptions {
		errors[rejected.path] = rejected.err
	}
	return errors
}

func noActiveSubscriptionsError() error {
	return errors.New("no active gNMI subscription paths; rejected paths will be reconsidered on reconnect after their retry interval or on configuration reload")
}
