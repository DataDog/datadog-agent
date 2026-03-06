// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package remotehostnameimpl provides a function to get the hostname from core agent.
package remotehostnameimpl

import (
	"context"
	"time"

	"github.com/avast/retry-go/v4"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"

	cache "github.com/patrickmn/go-cache"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	hostnameimpl "github.com/DataDog/datadog-agent/comp/core/hostname/impl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteHostImpl))
}

var cachKey = "hostname"

type remotehostimpl struct {
	cache  *cache.Cache
	ipc    ipc.Component
	config config.Component
	log    log.Component
}

type dependencies struct {
	fx.In
	IPC    ipc.Component
	Config config.Component
	Log    log.Component
}

func newRemoteHostImpl(deps dependencies) hostnamedef.Component {
	return &remotehostimpl{
		cache:  cache.New(defaultExpire, defaultPurge),
		ipc:    deps.IPC,
		config: deps.Config,
		log:    deps.Log,
	}
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
	h, err := r.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return h
}

func (r *remotehostimpl) GetWithProvider(ctx context.Context) (hostnamedef.Data, error) {
	h, err := r.Get(ctx)
	if err != nil {
		return hostnamedef.Data{}, err
	}
	return hostnamedef.Data{
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

		ipcAddress, err := pkgconfigsetup.GetIPCAddress(r.config)
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, r.config.GetString("cmd_port"), r.ipc.GetTLSClientConfig())
		if err != nil {
			return err
		}

		reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
		if err != nil {
			return err
		}

		r.log.Debugf("Acquired hostname from gRPC: %s", reply.Hostname)

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
		r.log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = hostnameimpl.GetFromConfig(ctx, r.config)
		if err != nil {
			return "", err
		}
	}
	r.log.Infof("Hostname is: %s", hostnameDetected)
	return hostnameDetected, nil
}
