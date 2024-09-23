// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"context"
	"math"
	"time"

	"golang.org/x/time/rate"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

const rateLimiterBurst = 1 // burst of 1 is sufficient since wait() requests permission to perform a single operation

type rateLimiter interface {
	wait(context.Context) error
	start()
	stop()
	markFailure()
	markSuccess()
}

// Rate limiter implementation for when rdnsquerier rate limiting is enabled
type rateLimiterImpl struct {
	config            *rdnsQuerierConfig
	logger            log.Component
	internalTelemetry *rdnsQuerierTelemetry

	limiter *rate.Limiter

	success chan struct{}
	failure chan struct{}
	exit    chan struct{}

	currentLimit       int
	consecutiveErrors  int
	timeOfNextIncrease time.Time
}

func newRateLimiter(config *rdnsQuerierConfig, logger log.Component, internalTelemetry *rdnsQuerierTelemetry) rateLimiter {
	if !config.rateLimiter.enabled {
		return &rateLimiterNone{}
	}
	return &rateLimiterImpl{
		config:            config,
		logger:            logger,
		internalTelemetry: internalTelemetry,

		limiter: rate.NewLimiter(rate.Limit(config.rateLimiter.limitPerSec), rateLimiterBurst),

		success: make(chan struct{}, config.workers),
		failure: make(chan struct{}, config.workers),
		exit:    make(chan struct{}),

		currentLimit: config.rateLimiter.limitPerSec,
	}
}

func (r *rateLimiterImpl) wait(ctx context.Context) error {
	return r.limiter.Wait(ctx)
}

func (r *rateLimiterImpl) start() {
	r.internalTelemetry.rateLimiterLimit.Set(float64(r.currentLimit))
	r.runLimitAdjuster()
}

func (r *rateLimiterImpl) stop() {
	close(r.exit)
}

func (r *rateLimiterImpl) runLimitAdjuster() {
	go func() {
		for {
			select {
			case <-r.success:
				r.processSuccess()
			case <-r.failure:
				r.processFailure()
			case <-r.exit:
				return
			}
		}
	}()
}

func (r *rateLimiterImpl) markFailure() {
	r.failure <- struct{}{}
}

func (r *rateLimiterImpl) markSuccess() {
	r.success <- struct{}{}
}

func (r *rateLimiterImpl) processFailure() {
	r.consecutiveErrors++
	if r.consecutiveErrors < r.config.rateLimiter.throttleErrorThreshold {
		return
	}

	r.timeOfNextIncrease = time.Now().Add(r.config.rateLimiter.recoveryInterval)

	if r.currentLimit <= r.config.rateLimiter.limitThrottledPerSec {
		return
	}

	r.currentLimit = r.config.rateLimiter.limitThrottledPerSec
	r.limiter.SetLimit(rate.Limit(r.currentLimit))

	r.internalTelemetry.rateLimiterLimit.Set(float64(r.currentLimit))
	r.logger.Debugf("rateLimiter.processFailure() consecutiveErrors: %d, set limit to %d\n", r.consecutiveErrors, r.currentLimit)
}

func (r *rateLimiterImpl) processSuccess() {
	r.consecutiveErrors = 0
	if r.currentLimit >= r.config.rateLimiter.limitPerSec {
		return
	}

	if r.timeOfNextIncrease.After(time.Now()) {
		return
	}

	adjust := int(math.Ceil(float64(r.config.rateLimiter.limitPerSec-r.config.rateLimiter.limitThrottledPerSec) / float64(r.config.rateLimiter.recoveryIntervals)))
	adjust = max(adjust, 1)
	newLimit := min(r.currentLimit+adjust, r.config.rateLimiter.limitPerSec)
	r.currentLimit = newLimit
	r.limiter.SetLimit(rate.Limit(r.currentLimit))

	r.internalTelemetry.rateLimiterLimit.Set(float64(r.currentLimit))
	r.logger.Debugf("rateLimiter.processSuccess() set limit to %d\n", r.currentLimit)

	r.timeOfNextIncrease = time.Now().Add(r.config.rateLimiter.recoveryInterval)
}

// No limit rate limiter for when rdnsquerier rate limiting is disabled
type rateLimiterNone struct{}

func (r *rateLimiterNone) wait(_ context.Context) error {
	return nil
}

func (r *rateLimiterNone) start() {
}
func (r *rateLimiterNone) stop() {
}

func (r *rateLimiterNone) markFailure() {
}
func (r *rateLimiterNone) markSuccess() {
}
