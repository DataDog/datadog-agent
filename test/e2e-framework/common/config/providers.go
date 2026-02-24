// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package config

import (
	"fmt"
	"sync"

	"github.com/pulumi/pulumi-awsx/sdk/v2/go/awsx"
	"github.com/pulumi/pulumi-command/sdk/go/command"
	"github.com/pulumi/pulumi-docker/sdk/v4/go/docker"
	"github.com/pulumi/pulumi-eks/sdk/v3/go/eks"
	"github.com/pulumi/pulumi-gcp/sdk/v7/go/gcp"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi-tls/sdk/v4/go/tls"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ProviderID string

const (
	ProviderRandom  ProviderID = "random"
	ProviderTLS     ProviderID = "tls"
	ProviderCommand ProviderID = "command"
	ProviderAWS     ProviderID = "aws"
	ProviderAWSX    ProviderID = "awsx"
	ProviderEKS     ProviderID = "eks"
	ProviderAzure   ProviderID = "azure"
	ProviderDocker  ProviderID = "docker"
	ProviderGCP     ProviderID = "gcp"
)

func dummyProvidersFactory() map[ProviderID]func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
	return map[ProviderID]func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error){
		ProviderRandom: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := random.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderTLS: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := tls.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderCommand: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := command.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderAWSX: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := awsx.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderEKS: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := eks.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderDocker: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := docker.NewProvider(ctx, name, nil)
			return provider, err
		},
		ProviderGCP: func(ctx *pulumi.Context, name string) (pulumi.ProviderResource, error) {
			provider, err := gcp.NewProvider(ctx, name, nil)
			return provider, err
		},
	}
}

type provider interface {
	WithProvider(providerID ProviderID) pulumi.InvokeOption
	WithProviders(providerID ...ProviderID) pulumi.ResourceOption
	RegisterProvider(providerID ProviderID, provider pulumi.ProviderResource)
	GetProvider(providerID ProviderID) pulumi.ProviderResource
}
type providerRegistry struct {
	ctx       *pulumi.Context
	providers map[ProviderID]pulumi.ProviderResource
	lock      *sync.Mutex
}

func newProviderRegistry(ctx *pulumi.Context) providerRegistry {
	return providerRegistry{
		ctx:       ctx,
		providers: make(map[ProviderID]pulumi.ProviderResource),
		lock:      &sync.Mutex{},
	}
}

func (p *providerRegistry) WithProvider(providerID ProviderID) pulumi.InvokeOption {
	return pulumi.Provider(p.GetProvider(providerID))
}

func (p *providerRegistry) WithProviders(providerID ...ProviderID) pulumi.ResourceOption {
	var providers []pulumi.ProviderResource
	for _, id := range providerID {
		providers = append(providers, p.GetProvider(id))
	}
	return pulumi.Providers(providers...)
}

func (p *providerRegistry) RegisterProvider(providerID ProviderID, provider pulumi.ProviderResource) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.providers[providerID] = provider
}

func (p *providerRegistry) GetProvider(providerID ProviderID) pulumi.ProviderResource {
	p.lock.Lock()
	defer p.lock.Unlock()

	if provider, found := p.providers[providerID]; found {
		return provider
	}

	if providerFactory, found := dummyProvidersFactory()[providerID]; found {
		provider, err := providerFactory(p.ctx, string(providerID))
		if err != nil {
			panic(fmt.Errorf("provider %s creation failed, err: %w", providerID, err))
		}

		p.providers[providerID] = provider
		return provider
	}

	panic(fmt.Sprintf("provider %s not registered", providerID))
}
