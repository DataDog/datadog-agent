// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package redis

import (
	"net"
	"crypto/tls"

	"github.com/go-redis/redis/v9"
)

// NewClient returns a new redis client.
func NewClient(serverAddress string, dialer *net.Dialer, withTLS TLSSetting) *redis.Client {
	opts := &redis.Options{
		Addr:   serverAddress,
	}
	if withTLS {
		opts.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	} else {
		opts.Dialer = dialer.DialContext
	}

	return redis.NewClient(opts)
}
