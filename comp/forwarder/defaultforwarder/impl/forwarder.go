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
	markPendingDelegatedAuthDomains(endpoints, config, delegatedAuth)

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
	// Override the DisableAPIKeyChecking only if WithFeatures was called
	disableOverride := params.APIKeyCheckingDisabledOverride()
	if disableAPIKeyChecking, ok := disableOverride.Get(); ok {
		options.DisableAPIKeyChecking = disableAPIKeyChecking
	}
	// set the secrets and delegated-auth components from the dependencies
	options.Secrets = secrets
	options.DelegatedAuth = delegatedAuth
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

// markPendingDelegatedAuthDomains recomputes HasPendingDelegatedAuth for each domain from
// delegatedAuth.IsManaged, which is the sole source of truth for whether a domain is currently
// WIF-managed. utils.GetMultipleEndpoints only sets an initial guess for HasPendingDelegatedAuth
// by checking whether the current api_key config value has a literal DELA(...) prefix - that
// guess doesn't account for configureAdditionalEndpointsDelegatedAuth rejecting a malformed
// directive or unsupported provider without ever registering it with delegated auth. Trusting
// the initial guess for a rejected directive would leave the domain permanently "pending" and
// its payloads would be retried forever instead of eventually dropping on repeated 403s. So this
// function always overwrites the flag with the freshly computed IsManaged result. This also
// covers the primary domain, which GetMultipleEndpoints never marks at all (it only inspects
// `additional_endpoints`).
func markPendingDelegatedAuthDomains(endpoints utils.EndpointDescriptorSet, config config.Component, delegatedAuth delegatedauth.Component) {
	if delegatedAuth == nil {
		return
	}
	primaryDomain := utils.GetInfraEndpoint(config)
	for domain, ed := range endpoints {
		managed := delegatedAuth.IsManaged(delegatedauth.Target{
			AdditionalEndpointsConfigKey: "additional_endpoints",
			AdditionalEndpointDomain:     domain,
		})
		if !managed && domain == primaryDomain {
			managed = delegatedAuth.IsManaged(delegatedauth.Target{APIKeyConfigKey: "api_key"})
		}
		ed.HasPendingDelegatedAuth = managed
		endpoints[domain] = ed
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
