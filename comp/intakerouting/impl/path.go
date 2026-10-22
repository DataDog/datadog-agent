// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package intakeroutingimpl

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

// Why the Agent does not reach a default host directly. They are also the values of the "reason"
// tag of the skipped_endpoints counter.
const (
	// pathPrivateAddress means the host resolves to at least one non-public address, as it does
	// behind PrivateLink. The endpoint is never routed.
	pathPrivateAddress = "private_address"
	// pathProxied means telemetry sent to the host goes through a proxy, which resolves it: the
	// Agent cannot tell how it is reached. The endpoint is never routed.
	pathProxied = "proxied"
	// pathLookupFailed means the host could not be resolved. The endpoint keeps its current host.
	pathLookupFailed = "lookup_failed"
)

// sharedAddressSpace is the shared address space of RFC 6598. Clouds allow it in private networks.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// pathChecker tells whether the Agent reaches a host directly, over public addresses.
type pathChecker interface {
	// check returns "" when the Agent reaches ep directly over public addresses. Otherwise it
	// returns one of the path reasons, and an error describing it.
	check(ctx context.Context, ep intakerouting.Endpoint) (string, error)
}

// netPathChecker checks the proxy settings of the transport telemetry is sent with, then resolves
// the host.
type netPathChecker struct {
	// proxy is the proxy function of the transport, or nil when no proxy is configured.
	proxy  func(*http.Request) (*url.URL, error)
	lookup func(ctx context.Context, network, host string) ([]netip.Addr, error)
}

func newNetPathChecker(transport *http.Transport) *netPathChecker {
	return &netPathChecker{proxy: transport.Proxy, lookup: net.DefaultResolver.LookupNetIP}
}

func (c *netPathChecker) check(ctx context.Context, ep intakerouting.Endpoint) (string, error) {
	if c.proxy != nil {
		req := &http.Request{Method: http.MethodPost, URL: &url.URL{Scheme: "https", Host: hostPort(ep)}, Header: http.Header{}}
		proxyURL, err := c.proxy(req)
		if err != nil {
			return pathProxied, fmt.Errorf("the proxy settings for %s are invalid: %w", ep.Host, err)
		}
		if proxyURL != nil {
			return pathProxied, fmt.Errorf("%s is reached through a proxy", ep.Host)
		}
	}

	// The host is under an official Datadog domain: resolve it as an absolute name, so that search
	// domains are never tried.
	addrs, err := c.lookup(ctx, "ip", strings.TrimSuffix(ep.Host, ".")+".")
	if err != nil {
		return pathLookupFailed, err
	}
	if len(addrs) == 0 {
		return pathLookupFailed, fmt.Errorf("%s has no address", ep.Host)
	}
	// A dual-stack PrivateLink endpoint can have a private IPv4 address and a global IPv6 one:
	// a single non-public address is enough.
	for _, addr := range addrs {
		if !isPublicAddress(addr) {
			return pathPrivateAddress, fmt.Errorf("%s resolves to the non-public address %s", ep.Host, addr)
		}
	}
	return "", nil
}

// isPublicAddress reports whether addr is a global unicast address outside of the private
// (RFC 1918, RFC 4193) and shared (RFC 6598) address spaces.
func isPublicAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsGlobalUnicast() && !addr.IsPrivate() && !sharedAddressSpace.Contains(addr)
}
