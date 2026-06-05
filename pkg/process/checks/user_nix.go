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

// LookupIDProbe wraps user.LookupId with an optional cache.
type LookupIDProbe struct {
	config pkgconfigmodel.Reader

	lookupIDCache *cache.Cache
	lookupID      func(uid string) (*user.User, error)
}

// NewLookupIDProbe returns a new LookupIDProbe from the config
func NewLookupIDProbe(coreConfig pkgconfigmodel.Reader) *LookupIDProbe {
	if coreConfig.GetBool("process_config.cache_lookupid") {
		log.Debug("Using cached calls to `user.LookupID`")
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

// LookupID returns the user.User for the given uid, using a cache if configured.
func (p *LookupIDProbe) LookupID(uid string) (*user.User, error) {
	if p.config.GetBool("process_config.cache_lookupid") {
		return p.lookupIDWithCache(uid)
	}
	return p.lookupID(uid)
}
