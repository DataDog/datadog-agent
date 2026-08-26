// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarderimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

type dependencies struct {
	Config        config.Component
	Log           log.Component
	Lc            compdef.Lifecycle
	Params        defaultforwarderdef.Params
	Secrets       secrets.Component
	DelegatedAuth delegatedauth.Component
}

type provides struct {
	Comp           defaultforwarderdef.Component
	StatusProvider status.InformationProvider
}

func newForwarder(dep dependencies) (provides, error) {
	options, err := createOptions(dep.Params, dep.Config, dep.Log, dep.Secrets, dep.DelegatedAuth)
	if err != nil {
		return provides{}, err
	}

	return NewForwarder(dep.Config, dep.Log, dep.Lc, true, options), nil
}

func createOptions(params defaultforwarderdef.Params, config config.Component, log log.Component, secrets secrets.Component, delegatedAuth delegatedauth.Component) (*Options, error) {
	var options *Options
	endpoints, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
		return nil, fmt.Errorf("Misconfiguration of agent endpoints: %s", err)
	}

	if !params.Resolver() {
		options, err = NewOptionsWithOPW(config, log, endpoints)
		if err != nil {
			log.Error("Error creating forwarder options: ", err)
			return nil, fmt.Errorf("Error creating forwarder options: %s", err)
		}
	} else {
		r, err := resolver.NewSingleDomainResolvers2(endpoints)
		if err != nil {
			log.Error("Error creating resolver: ", err)
			return nil, fmt.Errorf("Error creating resolver: %s", err)
		}
		options = NewOptionsWithResolvers(config, log, r)
	}
	// Attach after both branches, on the resolvers that were actually kept. NewOptionsWithOPW
	// builds its own set and can swap the infra resolver for a vector-diverted one, so attaching
	// inside the else branch alone left every binary that does not pass WithResolvers - the core
	// Agent, dogstatsd, serverless-init, otel-agent - with no providers at all, and a delegated
	// auth endpoint there produced no transactions and shipped nothing.
	attachCredentialProviders(options.DomainResolvers, endpoints, delegatedAuth, log)
	// Override the DisableAPIKeyChecking only if WithFeatures was called
	disableOverride := params.APIKeyCheckingDisabledOverride()
	if disableAPIKeyChecking, ok := disableOverride.Get(); ok {
		options.DisableAPIKeyChecking = disableAPIKeyChecking
	}
	// set the secrets component from the dependencies
	options.Secrets = secrets
	options.SetEnabledFeatures(params.EnabledFeatures())

	log.Infof("starting forwarder with %d endpoints", len(options.DomainResolvers))
	for _, resolver := range options.DomainResolvers {
		scrubbedKeys := []string{}
		for _, k := range resolver.GetAPIKeys() {
			scrubbedKeys = append(scrubbedKeys, scrubber.HideKeyExceptLastChars(k))
		}
		log.Infof("domain '%s' has %d keys: %s", resolver.GetBaseDomain(), len(scrubbedKeys), strings.Join(scrubbedKeys, ", "))
	}
	return options, nil
}

// attachCredentialProviders gives each resolver the delegated-auth providers registered for its
// destination, so a credential that resolves after startup reaches the send path with no rebuild
// and without the key ever entering the config tree.
//
// Each provider becomes its own authorization slot, which means a transaction is created for it
// even before it has a credential. That is deliberate: the transaction is then rescheduled by the
// retry queue until the credential lands, rather than the payload being dropped at creation.
func attachCredentialProviders(resolvers map[string]resolver.DomainResolver, endpoints utils.EndpointDescriptorSet, delegatedAuth delegatedauth.Component, log log.Component) {
	if delegatedAuth == nil {
		return
	}
	for _, ed := range endpoints {
		r, ok := resolvers[ed.BaseURL]
		if !ok {
			continue
		}
		var providers []resolver.CredentialProvider
		for _, keys := range ed.APIKeySet {
			providers = append(providers, delegatedAuth.ProvidersFor(keys.ConfigSettingPath, ed.BaseURL)...)
		}
		if len(providers) == 0 {
			continue
		}
		r.SetCredentialProviders(providers)
		log.Infof("domain '%s' has %d delegated-auth credential provider(s)", ed.BaseURL, len(providers))
	}
}

// NewForwarder returns a new forwarder component.
//
//nolint:revive
func NewForwarder(config config.Component, log log.Component, lc compdef.Lifecycle, ignoreLifeCycleError bool, options *Options) provides {
	forwarder := NewDefaultForwarder(config, log, options)

	lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			err := forwarder.Start()
			if ignoreLifeCycleError {
				return nil
			}
			return err
		},
		OnStop: func(context.Context) error { forwarder.Stop(); return nil }})

	return provides{
		Comp:           forwarder,
		StatusProvider: status.NewInformationProvider(statusProvider{config: config}),
	}
}

func newMockForwarder(config config.Component, log log.Component, secrets secrets.Component) provides {
	options, _ := NewOptions(config, log, nil)
	options.Secrets = secrets
	return provides{
		Comp: NewDefaultForwarder(config, log, options),
	}
}

// NewComponent is an exported wrapper around newForwarder for use with fx.
//
//nolint:revive
func NewComponent(dep dependencies) (provides, error) {
	return newForwarder(dep)
}

// NewMockForwarder provides a mock forwarder component for use with fx.
//
//nolint:revive
func NewMockForwarder(config config.Component, log log.Component, secrets secrets.Component) provides {
	return newMockForwarder(config, log, secrets)
}
