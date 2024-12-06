// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetReverseDnsForIP(t *testing.T) {
	t.Run("reverse dns lookup successful", func(t *testing.T) {
		lookupAddrFn = func(_ context.Context, _ string) ([]string, error) {
			return []string{"domain-a.com", "domain-b.com"}, nil
		}
		defer func() { lookupAddrFn = net.DefaultResolver.LookupAddr }()

		assert.Equal(t, "domain-a.com", GetReverseDNSForIP(net.ParseIP("1.2.3.4")))
		assert.Equal(t, "", GetReverseDNSForIP(nil))
	})
	t.Run("reverse dns lookup failure", func(t *testing.T) {
		lookupAddrFn = func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("some error")
		}
		defer func() { lookupAddrFn = net.DefaultResolver.LookupAddr }()

		assert.Equal(t, "1.2.3.4", GetReverseDNSForIP(net.ParseIP("1.2.3.4")))
		assert.Equal(t, "", GetReverseDNSForIP(nil))
	})
}

func Test_getHostname(t *testing.T) {
	t.Run("reverse dns lookup successful", func(t *testing.T) {
		lookupAddrFn = func(_ context.Context, _ string) ([]string, error) {
			return []string{"domain-a.com.", "domain-b.com."}, nil
		}
		defer func() { lookupAddrFn = net.DefaultResolver.LookupAddr }()

		assert.Equal(t, "domain-a.com", GetHostname("1.2.3.4"))
	})
	t.Run("reverse dns lookup failure", func(t *testing.T) {
		lookupAddrFn = func(_ context.Context, _ string) ([]string, error) {
			return nil, errors.New("some error")
		}
		defer func() { lookupAddrFn = net.DefaultResolver.LookupAddr }()

		assert.Equal(t, "1.2.3.4", GetHostname("1.2.3.4"))
	})
}
