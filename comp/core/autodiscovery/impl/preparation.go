// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package autodiscoveryimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// preparation owns a single default-provider/listener setup operation. Bare
// AutoConfig instances used by mocks do not install one: their providers are
// supplied explicitly, without reading the host's configuration or environment.
type preparation struct {
	mu         sync.Mutex
	initialize func()
	done       chan struct{}
	startCtx   context.Context
	stopped    bool
}

// start returns the same completion signal to every caller. The first caller's
// context bounds waiting, not setup itself: the setup APIs cannot be canceled.
func (p *preparation) start(ctx context.Context) (<-chan struct{}, context.Context, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return nil, nil, errors.New("autodiscovery preparation: already stopped")
	}
	if p.done == nil {
		p.done = make(chan struct{})
		p.startCtx = ctx
		ready := health.RegisterReadiness("ad-initialization")
		go func() {
			defer close(p.done)
			defer ready.Deregister() //nolint:errcheck // This goroutine owns the handle.
			p.initialize()
		}()
	}
	return p.done, p.startCtx, nil
}

func (p *preparation) wait(ctx context.Context) error {
	// Lazy preparation must not retain a waiter's deadline. Only explicit
	// preloading carries a startup context across to subsequent waiters.
	done, startCtx, err := p.start(context.Background())
	if err != nil {
		return err
	}
	// Prefer completed preparation even if the preload context has since expired.
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-startCtx.Done():
		return fmt.Errorf("autodiscovery preparation: %w", startCtx.Err())
	case <-ctx.Done():
		return fmt.Errorf("autodiscovery preparation: %w", ctx.Err())
	}
}

func (p *preparation) stop() {
	p.mu.Lock()
	p.stopped = true
	done := p.done
	p.mu.Unlock()
	if done != nil {
		// Do not tear down AutoConfig or its dependencies while setup uses them,
		// even if the shutdown context expires. Setup does not support cancellation.
		<-done
	}
}
