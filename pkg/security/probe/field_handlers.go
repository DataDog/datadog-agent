// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func bestGuessServiceTag(serviceValues []string) string {
	if len(serviceValues) == 0 {
		return ""
	}

	firstGuess := serviceValues[0]

	// first we sort base on len, biggest len first
	slices.SortFunc(serviceValues, func(a, b string) int {
		return cmp.Compare(len(b), len(a)) // reverse
	})

	// we then compare [i] and [i + 1] to check if [i + 1] is a prefix of [i]
	for i := 0; i < len(serviceValues)-1; i++ {
		if !strings.HasPrefix(serviceValues[i], serviceValues[i+1]) {
			// if it's not a prefix it means we have multiple disjoints services
			// we then return the first guess, closest in the process tree
			return firstGuess
		}
	}

	// we have a prefix chain, let's return the biggest one
	return serviceValues[0]
}

// getProcessService returns the service tag based on the process context
func getProcessService(config *config.Config, entry *model.ProcessCacheEntry) (string, bool) {
	var serviceValues []string

	// first search in the process context itself
	if entry.EnvsEntry != nil {
		if service := entry.EnvsEntry.Get(ServiceEnvVar); service != "" {
			serviceValues = append(serviceValues, service)
		}
	}

	inContainer := entry.ContainerID != ""

	// while in container check for each ancestor
	for ancestor := entry.Ancestor; ancestor != nil; ancestor = ancestor.Ancestor {
		if inContainer && ancestor.ContainerID == "" {
			break
		}

		if ancestor.EnvsEntry != nil {
			if service := ancestor.EnvsEntry.Get(ServiceEnvVar); service != "" {
				serviceValues = append(serviceValues, service)
			}
		}
	}

	if service := bestGuessServiceTag(serviceValues); service != "" {
		return service, true
	}

	return config.RuntimeSecurity.HostServiceName, false
}

// BaseFieldHandlers holds the base field handlers
type BaseFieldHandlers struct {
	config       *config.Config
	privateCIDRs eval.CIDRValues
	hostname     string
}

// NewBaseFieldHandlers creates a new BaseFieldHandlers
func NewBaseFieldHandlers(cfg *config.Config, hostname string) (*BaseFieldHandlers, error) {
	bfh := &BaseFieldHandlers{
		config:   cfg,
		hostname: hostname,
	}

	for _, cidr := range cfg.Probe.NetworkPrivateIPRanges {
		if err := bfh.privateCIDRs.AppendCIDR(cidr); err != nil {
			return nil, fmt.Errorf("error adding private IP range %s: %w", cidr, err)
		}
	}
	for _, cidr := range cfg.Probe.NetworkExtraPrivateIPRanges {
		if err := bfh.privateCIDRs.AppendCIDR(cidr); err != nil {
			return nil, fmt.Errorf("error adding extra private IP range %s: %w", cidr, err)
		}
	}

	return bfh, nil
}

// ResolveIsIPPublic resolves if the IP is public
func (bfh *BaseFieldHandlers) ResolveIsIPPublic(_ *model.Event, ipCtx *model.IPPortContext) bool {
	if !ipCtx.IsPublicResolved {
		ipCtx.IsPublic = !bfh.privateCIDRs.Contains(&ipCtx.IPNet)
		ipCtx.IsPublicResolved = true
	}
	return ipCtx.IsPublic
}

// ResolveHostname resolve the hostname
func (bfh *BaseFieldHandlers) ResolveHostname(_ *model.Event, _ *model.BaseEvent) string {
	return bfh.hostname
}

// ResolveService returns the service tag based on the process context
func (bfh *BaseFieldHandlers) ResolveService(ev *model.Event, e *model.BaseEvent) string {
	if e.Service != "" {
		return e.Service
	}

	entry, _ := ev.ResolveProcessCacheEntry()
	if entry == nil {
		return ""
	}

	service, ok := getProcessService(bfh.config, entry)
	if ok {
		e.Service = service
	}

	return service
}
