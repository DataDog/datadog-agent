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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//nolint:revive // TODO(PROC) Fix revive linter
type LookupIdProbe struct {
	config config.Reader

	lookupIdCache *cache.Cache
	//nolint:revive // TODO(PROC) Fix revive linter
	lookupId func(uid string) (*user.User, error)
}

// NewLookupIDProbe returns a new LookupIdProbe from the config
func NewLookupIDProbe(coreConfig config.Reader) *LookupIdProbe {
	if coreConfig.GetBool("process_config.cache_lookupid") {
		log.Debug("Using cached calls to `user.LookupID`")
	}
	return &LookupIdProbe{
		// Inject global logger and config to make it easy to use components
		config: coreConfig,

		lookupIdCache: cache.New(time.Hour, time.Hour), // Used by lookupIdWithCache
		lookupId:      user.LookupId,
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p *LookupIdProbe) lookupIdWithCache(uid string) (*user.User, error) {
	result, ok := p.lookupIdCache.Get(uid)
	if !ok {
		var err error
		u, err := p.lookupId(uid)
		if err == nil {
			p.lookupIdCache.SetDefault(uid, u)
		} else {
			p.lookupIdCache.SetDefault(uid, err)
		}
		return u, err
	}

	switch v := result.(type) {
	case *user.User:
		return v, nil
	case error:
		return nil, v
	default:
		return nil, log.Error("Unknown value cached in lookupIdCache for uid:", uid)
	}
}

//nolint:revive // TODO(PROC) Fix revive linter
func (p *LookupIdProbe) LookupId(uid string) (*user.User, error) {
	if p.config.GetBool("process_config.cache_lookupid") {
		return p.lookupIdWithCache(uid)
	}
	return p.lookupId(uid)
}
