// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package clusteridresolverimpl implements the cluster ID resolver component.
package clusteridresolverimpl

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v7"

	clusteridresolver "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

const healthCheckName = "cluster-id"

// Requires defines the dependencies of the resolver. The resolver must not
// depend on the tagger, because the tagger uses the resolver.
type Requires struct {
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Log       log.Component
}

// Provides defines the output of the resolver.
type Provides struct {
	Comp clusteridresolver.Component
}

// lookupFunc gets the cluster ID from one source.
type lookupFunc func(context.Context) (string, error)

// result holds the ID, or the error that prevents resolution.
type result struct {
	id  string
	err error
}

type resolver struct {
	log     log.Component
	backoff backoff.BackOff
	// result is never nil.
	result atomic.Pointer[result]
	// done is closed when result is final.
	done chan struct{}
}

// NewComponent returns a resolver that uses the first source that applies:
//  1. A valid DD_ORCHESTRATOR_CLUSTER_ID, on all agents.
//  2. Kubernetes, on the Cluster Agent.
//  3. The Cluster Agent API, on other agents that can reach a Cluster Agent.
//
// When no source applies, resolution is disabled.
func NewComponent(req Requires) Provides {
	resolver := newResolver(req.Log)
	if id, found := os.LookupEnv(clustername.ClusterIDEnv); found {
		if clustername.IsValidClusterID(id) {
			resolver.finish(id, nil)
			return Provides{Comp: resolver}
		}
		req.Log.Warnf("Ignoring %s: %q is not a valid cluster ID", clustername.ClusterIDEnv, id)
	}

	isClusterAgent := flavor.GetFlavor() == flavor.ClusterAgent
	lookup := selectLookup(req.Config, isClusterAgent)
	if lookup == nil {
		resolver.finish("", clusteridresolver.ErrDisabled)
		return Provides{Comp: resolver}
	}
	// The Cluster Agent must not become ready before it has the ID.
	resolver.start(req.Lifecycle, lookup, isClusterAgent)
	return Provides{Comp: resolver}
}

func (r *resolver) GetID() (string, error) {
	res := r.result.Load()
	return res.id, res.err
}

func (r *resolver) WaitForID(ctx context.Context) (string, error) {
	select {
	case <-r.done:
		return r.GetID()
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// selectLookup returns the ID source of this agent, or nil when it has none.
func selectLookup(cfg config.Component, isClusterAgent bool) lookupFunc {
	if isClusterAgent {
		if cfg.GetBool("cloud_foundry") {
			return nil
		}
		// resolveFromKubernetes is nil in builds without Kubernetes support.
		return resolveFromKubernetes
	}
	// CLC runners and agents with an explicit URL can reach the Cluster Agent
	// without Kubernetes detection.
	if cfg.GetBool("cluster_agent.enabled") &&
		(env.IsFeaturePresent(env.Kubernetes) || env.IsFeaturePresent(env.EKSFargate) ||
			cfg.GetBool("clc_runner_enabled") || cfg.GetString("cluster_agent.url") != "") {
		return resolveFromClusterAgent
	}
	return nil
}

func resolveFromClusterAgent(ctx context.Context) (string, error) {
	client, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return "", err
	}
	return client.GetKubernetesClusterID(ctx)
}

func newResolver(log log.Component) *resolver {
	policy := backoff.NewExponentialBackOff()
	policy.InitialInterval = time.Second
	policy.MaxInterval = 30 * time.Second
	resolver := &resolver{log: log, backoff: policy, done: make(chan struct{})}
	resolver.result.Store(&result{err: clusteridresolver.ErrNotResolved})
	return resolver
}

// start resolves the ID in the background while the component runs. When
// gateHealth is true, the startup and liveness probes fail until the ID is
// resolved. The liveness probe also gates readiness.
func (r *resolver) start(lc compdef.Lifecycle, lookup lookupFunc, gateHealth bool) {
	var handles []*health.Handle
	if gateHealth {
		// Register now, because other start hooks can serve the probes first.
		handles = []*health.Handle{health.RegisterStartup(healthCheckName), health.RegisterLiveness(healthCheckName)}
	}

	var cancel context.CancelFunc
	lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			// The start context expires after startup, thus resolution uses its own context.
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())
			go func() {
				id, err := r.run(ctx, lookup)
				for _, handle := range handles {
					if deregisterErr := handle.Deregister(); deregisterErr != nil {
						r.log.Warnf("Cannot deregister the %s health check: %v", healthCheckName, deregisterErr)
					}
				}
				r.finish(id, err)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			cancel()
			select {
			case <-r.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
}

// run calls lookup until it returns a valid ID or until ctx is canceled.
func (r *resolver) run(ctx context.Context, lookup lookupFunc) (string, error) {
	return backoff.Retry(
		ctx,
		func() (string, error) {
			id, err := lookup(ctx)
			if err == nil && !clustername.IsValidClusterID(id) {
				err = fmt.Errorf("invalid cluster ID %q", id)
			}
			return id, err
		},
		backoff.WithBackOff(r.backoff),
		backoff.WithMaxElapsedTime(0),
		backoff.WithNotify(func(err error, next time.Duration) {
			r.result.Store(&result{err: err})
			r.log.Warnf("Cannot resolve the cluster ID, next try in %s: %v", next, err)
		}),
	)
}

// finish stores the final result and releases the waiters.
func (r *resolver) finish(id string, err error) {
	if err != nil {
		r.result.Store(&result{err: err})
	} else {
		clustername.SetClusterID(id)
		r.result.Store(&result{id: id})
		r.log.Infof("Cluster ID is %s", id)
	}
	close(r.done)
}
