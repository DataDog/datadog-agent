// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

// LimiterOptions contains the rate limits for the different AWS API services.
type LimiterOptions struct {
	EC2Rate          rate.Limit
	EBSListBlockRate rate.Limit
	EBSGetBlockRate  rate.Limit
	DefaultRate      rate.Limit
}

// Limiter is a rate limiter for AWS API calls.
type Limiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	opts     LimiterOptions
}

// NewLimiter returns a new Limiter with the given options.
func NewLimiter(opts LimiterOptions) *Limiter {
	return &Limiter{
		limiters: make(map[string]*rate.Limiter),
		opts:     opts,
	}
}

// Get returns a rate limiter for the given accountID, region, service and action.
func (l *Limiter) Get(accountID, region, service, action string) *rate.Limiter {
	var limit rate.Limit
	switch service {
	case "ec2":
		switch {
		// reference: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/throttling.html#throttling-limits
		case strings.HasPrefix(action, "Describe"), strings.HasPrefix(action, "Get"):
			limit = l.opts.EC2Rate
		default:
			limit = l.opts.EC2Rate / 4.0
		}
	case "ebs":
		switch action {
		case "getblock":
			limit = l.opts.EBSGetBlockRate
		case "listblocks", "changedblocks":
			limit = l.opts.EBSListBlockRate
		}
	case "s3", "imds":
		limit = 0.0 // no rate limiting
	default:
		limit = l.opts.DefaultRate
	}
	if limit == 0.0 {
		return nil // no rate limiting
	}
	key := accountID + region + service + action
	l.mu.Lock()
	ll, ok := l.limiters[key]
	if !ok {
		ll = rate.NewLimiter(limit, 1)
		l.limiters[key] = ll
	}
	l.mu.Unlock()
	return ll
}
