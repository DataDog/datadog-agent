// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package remotehostnameimpl provides a function to get the hostname from core agent.
package remotehostnameimpl

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/avast/retry-go/v4"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	cache "github.com/patrickmn/go-cache"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
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
	// defaultMaxAttempts is the default number of times we try to get the
	// hostname from the core-agent before bailing out.
	defaultMaxAttempts = 6
)

// Option configures the remote hostname component retry behavior.
type Option func(*remotehostimpl)

// WithMaxAttempts sets the maximum number of retry attempts to reach
// the core-agent for hostname resolution.
func WithMaxAttempts(maxAttempts uint) Option {
	return func(r *remotehostimpl) { r.maxAttempts = maxAttempts }
}

// WithMaxRetryDelay caps the exponential backoff between retry attempts.
func WithMaxRetryDelay(maxRetryDelay time.Duration) Option {
	return func(r *remotehostimpl) { r.maxRetryDelay = maxRetryDelay }
}

// options wraps []Option for fx injection.
type options []Option

// Module defines the fx options for this component.
func Module(opts ...Option) fxutil.Module {
	return fxutil.Component(
		fx.Supply(options(opts)),
		fx.Provide(newRemoteHostImpl))
}

var cachKey = "hostname"

type remotehostimpl struct {
	cache         *cache.Cache
	ipc           ipc.Component
	maxAttempts   uint
	maxRetryDelay time.Duration
}

type dependencies struct {
	fx.In
	IPC  ipc.Component
	Opts options
}

func newRemoteHostImpl(deps dependencies) hostnameinterface.Component {
	r := &remotehostimpl{
		cache:       cache.New(defaultExpire, defaultPurge),
		ipc:         deps.IPC,
		maxAttempts: defaultMaxAttempts,
	}
	for _, o := range deps.Opts {
		o(r)
	}
	return r
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

func (r *remotehostimpl) GetWithProvider(ctx context.Context) (hostnameinterface.Data, error) {
	h, err := r.Get(ctx)
	if err != nil {
		return hostnameinterface.Data{}, err
	}
	return hostnameinterface.Data{
		Hostname: h,
		Provider: "remote",
	}, nil
}

// getHostnameWithContext attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context.
func (r *remotehostimpl) getHostnameWithContext(ctx context.Context) (string, error) {
	ipcPort, err := strconv.Atoi(pkgconfigsetup.GetIPCPort())
	if err != nil || ipcPort <= 0 {
		return "", fmt.Errorf("IPC port is disabled (%s), skipping core-agent hostname lookup", pkgconfigsetup.GetIPCPort())
	}

	var hostname string

	retryOpts := []retry.Option{
		retry.LastErrorOnly(true),
		retry.Attempts(r.maxAttempts),
		retry.Context(ctx),
	}
	if r.maxRetryDelay > 0 {
		retryOpts = append(retryOpts, retry.MaxDelay(r.maxRetryDelay))
	}

	err = retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, pkgconfigsetup.GetIPCPort(), r.ipc.GetTLSClientConfig())
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
	}, retryOpts...)
	return hostname, err
}

// getHostnameWithContextAndFallback attempts to acquire a hostname by connecting to the
// core agent's gRPC endpoints extending the given context, or falls back to local resolution
func (r *remotehostimpl) getHostnameWithContextAndFallback(ctx context.Context) (string, error) {
	hostnameDetected, err := r.getHostnameWithContext(ctx)
	if err != nil {
		log.Warnf("Could not resolve hostname from core-agent: %v", err)
		hostnameDetected, err = hostname.Get(ctx)
		if err != nil {
			return "", err
		}
	}
	log.Infof("Hostname is: %s", hostnameDetected)
	return hostnameDetected, nil
}
