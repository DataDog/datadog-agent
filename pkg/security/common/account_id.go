// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/azure"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/gce"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	accountIDQueryTimeout = 5 * time.Second
	maxAccountIDAttempts  = 3
	accountIDRetryBackoff = 5 * time.Second
)

var errNoCloudProvider = errors.New("no cloud provider detected")

type cloudProviderDetector struct {
	name              string
	accountIDName     string
	callback          func(context.Context) bool
	accountIDCallback func(context.Context) (string, error)
}

func defaultCloudProviderDetectors() []cloudProviderDetector {
	return []cloudProviderDetector{
		{name: ec2.CloudProviderName, accountIDName: "account_id", callback: ec2.IsRunningOn, accountIDCallback: ec2.GetAccountID},
		{name: gce.CloudProviderName, accountIDName: "project_id", callback: gce.IsRunningOn, accountIDCallback: gce.GetProjectID},
		{name: azure.CloudProviderName, accountIDName: "subscription_id", callback: azure.IsRunningOn, accountIDCallback: azure.GetSubscriptionID},
	}
}

func queryAccountID(ctx context.Context) (string, string, error) {
	return queryAccountIDFromDetectors(ctx, defaultCloudProviderDetectors())
}

func queryAccountIDFromDetectors(ctx context.Context, detectors []cloudProviderDetector) (string, string, error) {
	for _, cloudDetector := range detectors {
		if cloudDetector.callback(ctx) {
			log.Infof("Cloud provider %s detected", cloudDetector.name)

			accountID, err := cloudDetector.accountIDCallback(ctx)
			if err != nil {
				return "", "", fmt.Errorf("could not detect cloud provider account ID: %w", err)
			}

			log.Infof("Detecting account id from %s cloud provider: %+q", cloudDetector.name, accountID)

			return cloudDetector.accountIDName, accountID, nil
		}
	}

	return "", "", errNoCloudProvider
}

type accountIDTagResolver struct {
	mu           sync.Mutex
	resolved     atomic.Bool
	value        string
	attempts     int
	maxAttempts  int
	retryBackoff time.Duration
	lastAttempt  time.Time
	detectors    []cloudProviderDetector
}

func newAccountIDTagResolver(detectors []cloudProviderDetector, maxAttempts int, retryBackoff time.Duration) *accountIDTagResolver {
	return &accountIDTagResolver{
		maxAttempts:  maxAttempts,
		retryBackoff: retryBackoff,
		detectors:    detectors,
	}
}

func (r *accountIDTagResolver) queryAccountIDTag() string {
	if r.resolved.Load() {
		return r.value
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.resolved.Load() {
		return r.value
	}

	if !r.lastAttempt.IsZero() && time.Since(r.lastAttempt) < r.retryBackoff {
		return r.value
	}

	ctx, cancel := context.WithTimeout(context.Background(), accountIDQueryTimeout)
	defer cancel()

	r.attempts++

	tagName, tagValue, err := queryAccountIDFromDetectors(ctx, r.detectors)
	if err == nil {
		r.value = tagName + ":" + tagValue
		r.resolved.Store(true)
		return r.value
	}

	if errors.Is(err, errNoCloudProvider) {
		log.Debugf("failed to query account id: %v", err)
		r.resolved.Store(true)
		return ""
	}

	if r.attempts >= r.maxAttempts {
		log.Warnf("failed to query account id after %d attempts: %v", r.attempts, err)
		r.resolved.Store(true)
		return ""
	}

	r.lastAttempt = time.Now()
	log.Debugf("failed to query account id (attempt %d/%d): %v", r.attempts, r.maxAttempts, err)
	return ""
}

var accountIDTagCache = newAccountIDTagResolver(
	defaultCloudProviderDetectors(),
	maxAccountIDAttempts,
	accountIDRetryBackoff,
)

// QueryAccountIDTag returns the account id tag matching the current deployment
func QueryAccountIDTag() string {
	return accountIDTagCache.queryAccountIDTag()
}
