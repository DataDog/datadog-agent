// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"os/user"
	"time"

	"github.com/patrickmn/go-cache"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LookupIDProbe resolves users with an optional per-UID cache.
type LookupIDProbe struct {
	config pkgconfigmodel.Reader

	lookupIDCache *cache.Cache
	lookupID      func(uid string) (*user.User, error)
}

// NewLookupIDProbe returns a new LookupIDProbe from the config
func NewLookupIDProbe(coreConfig pkgconfigmodel.Reader) *LookupIDProbe {
	if coreConfig.GetBool("process_config.cache_lookupid") {
		log.Debug("Using cached user lookups")
	}
	return &LookupIDProbe{
		// Inject global logger and config to make it easy to use components
		config: coreConfig,

		lookupIDCache: cache.New(time.Hour, time.Hour), // Used by lookupIDWithCache
		lookupID:      user.LookupId,
	}
}

func (p *LookupIDProbe) lookupIDWithCache(uid string) (*user.User, error) {
	result, ok := p.lookupIDCache.Get(uid)
	if !ok {
		var err error
		u, err := p.lookupIDUncached(uid)
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

func (p *LookupIDProbe) lookupIDUncached(uid string) (*user.User, error) {
	if u := lookupHostUser(uid); u != nil {
		return u, nil
	}
	return p.lookupID(uid)
}

// LookupID returns the cached user or error when caching is enabled and the UID is cached.
// Otherwise, it prefers HOST_ETC/passwd and falls back to the local user database
// when HOST_ETC is unset, the file cannot be read, or no matching entry is found.
// The optional cache retains results and errors for one hour, including local fallbacks.
func (p *LookupIDProbe) LookupID(uid string) (*user.User, error) {
	if p.config.GetBool("process_config.cache_lookupid") {
		return p.lookupIDWithCache(uid)
	}
	return p.lookupIDUncached(uid)
}
