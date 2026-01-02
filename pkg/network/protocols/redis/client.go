// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf && test

// Package redis implements USM's Redis monitoring, as well as provide
// helpers used in tests.
package redis

import (
	"crypto/tls"
	"errors"
	"net"
	"slices"

	"github.com/redis/go-redis/v9"
)

var errProtocolVersionNotSupported = errors.New("protocol version not supported")
var supportedProtocolVersions = []int{2, 3}

// NewClient returns a new redis client.
func NewClient(serverAddress string, dialer *net.Dialer, enableTLS bool, protocolVersion int) (*redis.Client, error) {

	if err := verifyProtocolVersion(protocolVersion); err != nil {
		return nil, err
	}

	opts := &redis.Options{
		Addr:     serverAddress,
		Protocol: protocolVersion,
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

	return redis.NewClient(opts), nil
}

func verifyProtocolVersion(protocolVersion int) error {
	if slices.Contains(supportedProtocolVersions, protocolVersion) {
		return nil
	}
	return errProtocolVersionNotSupported
}
