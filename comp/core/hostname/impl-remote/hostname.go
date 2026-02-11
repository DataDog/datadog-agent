// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package remoteimpl provides a function to get the hostname from core agent.
package remoteimpl

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	cache "github.com/patrickmn/go-cache"
)

const (
	defaultExpire = 15 * time.Minute
	defaultPurge  = 30 * time.Second
	// AgentCachePrefix is the common root to use to prefix all the cache
	// keys for any value regarding the Agent
	AgentCachePrefix = "agent"

	// encapsulate the cache module for easy refactoring

	// NoExpiration maps to go-cache corresponding value
	NoExpiration = cache.NoExpiration
	// maxAttempts is the maximum number of times we try to get the hostname
	// from the core-agent before bailing out.
	maxAttempts = 6
)

// Requires declares the input types to the remote hostname component constructor
type Requires struct {
	Lc     compdef.Lifecycle
	IPC    ipc.Component
	Config config.Component
}

// Provides declares the output types from the remote hostname component constructor
type Provides struct {
	compdef.Out

	Comp hostname.Component
}

var cachKey = "hostname"

type remotehostimpl struct {
	cache  *cache.Cache
	ipc    ipc.Component
	config config.Component
}

// NewComponent creates a new remote hostname component following the standard component pattern.
func NewComponent(reqs Requires) (Provides, error) {
	svc := &remotehostimpl{
		cache:  cache.New(defaultExpire, defaultPurge),
		ipc:    reqs.IPC,
		config: reqs.Config,
	}

	return Provides{Comp: svc}, nil
}

func (r *remotehostimpl) Get(ctx context.Context) (string, error) {
	if hostname, found := r.cache.Get(cachKey); found {
		return hostname.(string), nil
	}
	hostname, err := r.getHostnameWithContextAndFallback(ctx)
	if err != nil {
		return "", err
	}
	r.cache.Set(cachKey, hostname, NoExpiration)
	return hostname, nil
}

func (r *remotehostimpl) GetSafe(ctx context.Context) string {
	h, _ := r.Get(ctx)
	return h
}

func (r *remotehostimpl) GetWithProvider(ctx context.Context) (hostname.Data, error) {
	h, err := r.Get(ctx)
	if err != nil {
		return hostname.Data{}, err
	}
	return hostname.Data{
		Hostname: h,
		Provider: "remote",
	}, nil
}

// getHostnameWithContext attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context.
func (r *remotehostimpl) getHostnameWithContext(ctx context.Context) (string, error) {
	var hostname string
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		client, err := grpc.GetDDAgentClient(ctx, r.config.GetString("cmd_host"), r.config.GetString("cmd_port"), r.ipc.GetTLSClientConfig())
		if err != nil {
			return err
		}

		reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
		if err != nil {
			return err
		}

		log.Debugf("Acquired hostname from gRPC: %s", reply.Hostname)

		hostname = reply.Hostname
		return nil
	}, retry.LastErrorOnly(true), retry.Attempts(maxAttempts), retry.Context(ctx))
	return hostname, err
}

// getHostnameWithContextAndFallback attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context, or falls back to local resolution
func (r *remotehostimpl) getHostnameWithContextAndFallback(ctx context.Context) (string, error) {
	hostnameDetected, err := r.getHostnameWithContext(ctx)
	if err != nil {
		log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = pkghostname.Get(ctx)
		if err != nil {
			return "", err
		}
	}
	log.Infof("Hostname is: %s", hostnameDetected)
	return hostnameDetected, nil
}
