// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"os/user"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/userresolver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LookupIDProbe wraps user.LookupId with an optional cache.
type LookupIDProbe struct {
	config pkgconfigmodel.Reader

	lookupIDCache *cache.Cache
	lookupID      func(uid string) (*user.User, error)
	resolverOnce  sync.Once
	resolver      *userresolver.Resolver
}

// NewLookupIDProbe returns a new LookupIDProbe from the config
func NewLookupIDProbe(coreConfig pkgconfigmodel.Reader) *LookupIDProbe {
	if coreConfig.GetBool("process_config.cache_lookupid") {
		log.Debug("Using cached calls to `user.LookupID`")
	}
	probe := &LookupIDProbe{
		// Inject global logger and config to make it easy to use components
		config: coreConfig,

		lookupIDCache: cache.New(time.Hour, time.Hour), // Used by lookupIDWithCache
		lookupID:      user.LookupId,
	}
	probe.initResolver()
	return probe
}

func (p *LookupIDProbe) lookupIDWithCache(uid string) (*user.User, error) {
	result, ok := p.lookupIDCache.Get(uid)
	if !ok {
		var err error
		u, err := p.lookupID(uid)
		if err == nil {
			p.lookupIDCache.SetDefault(uid, u)
		} else {
			p.lookupIDCache.SetDefault(uid, err)
		}
		return u, err
	}

	switch v := result.(type) {
	case *user.User:
		return v, nil
	case error:
		return nil, v
	default:
		return nil, log.Error("Unknown value cached in lookupIDCache for uid:", uid)
	}
}

func (p *LookupIDProbe) lookupIDFallback(uid string) (*user.User, error) {
	if p.config != nil && p.config.GetBool("process_config.cache_lookupid") {
		return p.lookupIDWithCache(uid)
	}
	if p.lookupID != nil {
		return p.lookupID(uid)
	}
	return user.LookupId(uid)
}

func (p *LookupIDProbe) initResolver() {
	p.resolverOnce.Do(func() {
		p.resolver = userresolver.New(p.lookupIDFallback)
	})
}

// LookupID returns the user.User for the given uid, preferring HOST_ETC/passwd
// and using the configured cache only for the local user database fallback.
func (p *LookupIDProbe) LookupID(uid string) (*user.User, error) {
	p.initResolver()
	return p.resolver.LookupID(uid)
}
