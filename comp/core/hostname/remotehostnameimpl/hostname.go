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

	"github.com/DataDog/datadog-agent/pkg/config"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	cache "github.com/patrickmn/go-cache"
	"go.uber.org/fx"
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
	cache *cache.Cache
}

func newRemoteHostImpl() hostnameinterface.Component {
	return &remotehostimpl{
		cache: cache.New(defaultExpire, defaultPurge),
	}
}

func (r *remotehostimpl) Get(ctx context.Context) (string, error) {
	if hostname, found := r.cache.Get(cachKey); found {
		return hostname.(string), nil
	}
	hostname, err := getHostnameWithContextAndFallback(ctx)
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
func getHostnameWithContext(ctx context.Context) (string, error) {
	var hostname string
	err := retry.Do(func() error {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		ipcAddress, err := config.GetIPCAddress()
		if err != nil {
			return err
		}

		client, err := grpc.GetDDAgentClient(ctx, ipcAddress, config.GetIPCPort())
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
func getHostnameWithContextAndFallback(ctx context.Context) (string, error) {
	hostnameDetected, err := getHostnameWithContext(ctx)
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
