// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package redis TODO comment
package redis

import (
	"net"

	"github.com/go-redis/redis/v9"
)

// NewClient exported function should have comment or be unexported
func NewClient(serverAddress string, dialer *net.Dialer) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:   serverAddress,
		Dialer: dialer.DialContext,
	})
}
