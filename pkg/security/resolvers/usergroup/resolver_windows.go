// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package usergroup holds usergroup related files
package usergroup

import (
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/security/utils"

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	numberAllowedUserResolution = 5
	userResolutionCacheSize     = 64
)

// Resolver defines a resolver
type Resolver struct {
	//user lookup cache and rate limiter
	usersCache        *lru.Cache[string, string]
	userLookupLimiter *utils.Limiter[uint64]
}

// NewResolver returns a new process resolver
func NewResolver() (*Resolver, error) {

	r := &Resolver{}

	// create a cache for the username resolution
	cache, err := lru.New[string, string](userResolutionCacheSize)
	if err != nil {
		return nil, err
	}
	r.usersCache = cache

	// create a rate limiter that numberAllowedUserResolution
	limiter, err := utils.NewLimiter[uint64](1, numberAllowedUserResolution, time.Second)
	if err != nil {
		return nil, err
	}
	r.userLookupLimiter = limiter
	return p, nil
}

// GetUser returns the username
func (r *Resolver) GetUser(ownerSidString string) (name string) {

	username, found := r.usersCache.Get(ownerSidString)
	if found {
		// If username was already resolved
		return username
	}

	if !p.userLookupLimiter.Allow(1) {
		// If the username was not already resolved but the limit is hit, return an empty string
		return ""
	}
	// The limit is not hit and we need to resolve the username
	sid, err := windows.StringToSid(ownerSidString)
	if err != nil {
		r.usersCache.Add(ownerSidString, "")
		return ""
	}
	user, domain, _, err := sid.LookupAccount("")
	if err != nil {
		r.usersCache.Add(ownerSidString, "")
		return ""
	}
	r.usersCache.Add(ownerSidString, domain+"\\"+user)
	return domain + "\\" + user
}
