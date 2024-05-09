// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package traceroute

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getDestinationHostname(t *testing.T) {
	t.Run("reverse dns lookup successful", func(t *testing.T) {
		lookupAddrFn = func(addr string) ([]string, error) {
			return []string{"domain-a.com", "domain-b.com"}, nil
		}
		defer func() { lookupAddrFn = net.LookupAddr }()

		assert.Equal(t, "domain-a.com", getDestinationHostname("1.2.3.4"))
		assert.Equal(t, "not-an-ip", getDestinationHostname("not-an-ip"))
	})
	t.Run("reverse dns lookup failure", func(t *testing.T) {
		lookupAddrFn = func(addr string) ([]string, error) {
			return nil, errors.New("some error")
		}
		defer func() { lookupAddrFn = net.LookupAddr }()

		assert.Equal(t, "1.2.3.4", getDestinationHostname("1.2.3.4"))
		assert.Equal(t, "not-an-ip", getDestinationHostname("not-an-ip"))
	})
}

func Test_getHostname(t *testing.T) {
	t.Run("reverse dns lookup successful", func(t *testing.T) {
		lookupAddrFn = func(addr string) ([]string, error) {
			return []string{"domain-a.com.", "domain-b.com."}, nil
		}
		defer func() { lookupAddrFn = net.LookupAddr }()

		assert.Equal(t, "domain-a.com", getHostname("1.2.3.4"))
	})
	t.Run("reverse dns lookup failure", func(t *testing.T) {
		lookupAddrFn = func(addr string) ([]string, error) {
			return nil, errors.New("some error")
		}
		defer func() { lookupAddrFn = net.LookupAddr }()

		assert.Equal(t, "1.2.3.4", getHostname("1.2.3.4"))
	})
}
