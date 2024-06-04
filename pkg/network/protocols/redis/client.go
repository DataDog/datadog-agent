// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

import (
	"crypto/tls"
	"net"

	"github.com/go-redis/redis/v9"
)

// NewClient returns a new redis client.
func NewClient(serverAddress string, dialer *net.Dialer, enableTLS bool) *redis.Client {
	opts := &redis.Options{
		Addr: serverAddress,
	}

	if enableTLS {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		opts.Dialer = tlsDialer.DialContext
	} else {
		opts.Dialer = dialer.DialContext
	}

	return redis.NewClient(opts)
}
