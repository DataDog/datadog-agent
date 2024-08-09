// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"fmt"
	"net"
)

type resolver interface {
	lookup(string) (string, error)
}

func newResolver(config *rdnsQuerierConfig) resolver {
	return &resolverImpl{
		config: config,
	}
}

// Resolver implementation for default resolver
type resolverImpl struct {
	config *rdnsQuerierConfig
}

func (r *resolverImpl) lookup(addr string) (string, error) {
	// net.LookupAddr() can return both a non-zero length slice of hostnames and an error, but when
	// using the host C library resolver at most one result will be returned.  So for now, since
	// specifying other DNS resolvers is not supported, if we get an error we know that no valid
	// hostname was returned.
	hostnames, err := net.LookupAddr(addr)
	if err != nil {
		return "", err
	}

	// if !err then there should be at least one, but just to be safe
	if len(hostnames) == 0 {
		return "", fmt.Errorf("net.LookupAddr returned no hostnames for IP address %v", addr)
	}

	return hostnames[0], nil
}
