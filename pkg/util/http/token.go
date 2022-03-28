// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"context"
	"sync"
	"time"
)

// APITokenRenewCallback represents the callback type to fetch a token
type APITokenRenewCallback func(context.Context) (string, time.Time, error)

// APIToken is an API token with auto renew when expired
type APIToken struct {
	Value          string
	ExpirationDate time.Time
	renewCallback  APITokenRenewCallback

	sync.RWMutex
}

// NewAPIToken returns a new APIToken
func NewAPIToken(cb APITokenRenewCallback) *APIToken {
	return &APIToken{
		renewCallback: cb,
	}
}

// Get returns the token value
func (token *APIToken) Get(ctx context.Context) (string, error) {
	token.RLock()
	// The token renewal window is open, refreshing the token
	if time.Now().Before(token.ExpirationDate) {
		val := token.Value
		token.RUnlock()
		return val, nil
	}
	token.RUnlock()

	token.Lock()
	defer token.Unlock()
	// Token has been refreshed by another caller
	if time.Now().Before(token.ExpirationDate) {
		return token.Value, nil
	}

	value, expirationDate, err := token.renewCallback(ctx)
	if err != nil {
		token.ExpirationDate = time.Now()
		return "", err
	}

	token.Value = value
	token.ExpirationDate = expirationDate
	return token.Value, nil
}
