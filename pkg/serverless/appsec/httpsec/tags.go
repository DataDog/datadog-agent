// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:deadcode,unused
package httpsec

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// envClientIPHeader is the name of the env var used to specify the IP header to be used for client IP collection.
const envClientIPHeader = "DD_TRACE_CLIENT_IP_HEADER"

var (
	ipv6SpecialNetworks = []*netip.Prefix{
		ippref("fec0::/10"), // site local
	}
	clientIPHeader string

	defaultIPHeaders = []string{
		"x-forwarded-for",
		"x-real-ip",
		"x-client-ip",
		"x-forwarded",
		"x-cluster-client-ip",
		"forwarded-for",
		"forwarded",
		"via",
		"true-client-ip",
	}

	// List of HTTP headers we collect and send.
	collectedHTTPHeaders = append(defaultIPHeaders,
		"host",
		"content-length",
		"content-type",
		"content-encoding",
		"content-language",
		"forwarded",
		"user-agent",
		"accept",
		"accept-encoding",
		"accept-language")
)

func init() {
	// Required by sort.SearchStrings
	sort.Strings(collectedHTTPHeaders[:])

	// Read the IP-parsing configuration
	clientIPHeader = strings.ToLower(os.Getenv(envClientIPHeader))
}

// span interface expected by this package to set span tags.
type span interface {
	SetMetaTag(tag string, value string)
	SetMetricsTag(tag string, value float64)
	GetMetaTag(tag string) (value string, exists bool)
}

// setAppSecEnabledTags sets the AppSec-specific span tags that are expected to
// be in service entry span when AppSec is enabled.
func setAppSecEnabledTags(span span) {
	span.SetMetricsTag("_dd.appsec.enabled", 1)
}

// setEventSpanTags sets the security event span tags into the service entry span.
func setEventSpanTags(span span, events json.RawMessage) error {
	// Set the appsec event span tag
	val, err := makeEventsTagValue(events)
	if err != nil {
		return err
	}
	span.SetMetaTag("_dd.appsec.json", string(val))
	// Set the appsec.event tag needed by the appsec backend
	span.SetMetaTag("appsec.event", "true")
	return nil
}

// Create the value of the security events tag.
func makeEventsTagValue(events json.RawMessage) (json.RawMessage, error) {
	// Create the structure to use in the `_dd.appsec.json` span tag.
	v := struct {
		Triggers json.RawMessage `json:"triggers"`
	}{Triggers: events}
	tag, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("unexpected error while serializing the appsec event span tag: %v", err)
	}
	return tag, nil
}

// setSecurityEventsTags sets the AppSec-specific span tags when security events were found.
func setSecurityEventsTags(span span, events json.RawMessage, headers, respHeaders map[string][]string) {
	if err := setEventSpanTags(span, events); err != nil {
		log.Errorf("appsec: unexpected error while creating the appsec event tags: %v", err)
		return
	}
	for h, v := range normalizeHTTPHeaders(headers) {
		span.SetMetaTag("http.request.headers."+h, v)
	}
	for h, v := range normalizeHTTPHeaders(respHeaders) {
		span.SetMetaTag("http.response.headers."+h, v)
	}
}

// normalizeHTTPHeaders returns the HTTP headers following Datadog's
// normalization format.
func normalizeHTTPHeaders(headers map[string][]string) (normalized map[string]string) {
	if len(headers) == 0 {
		return nil
	}
	normalized = make(map[string]string)
	for k, v := range headers {
		k = strings.ToLower(k)
		if i := sort.SearchStrings(collectedHTTPHeaders[:], k); i < len(collectedHTTPHeaders) && collectedHTTPHeaders[i] == k {
			normalized[k] = strings.Join(v, ",")
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// ippref returns the IP network from an IP address string s. If not possible, it returns nil.
func ippref(s string) *netip.Prefix {
	if prefix, err := netip.ParsePrefix(s); err == nil {
		return &prefix
	}
	return nil
}

// setClientIPTags sets the http.client_ip, http.request.headers.*, and
// network.client.ip span tags according to the request headers and remote
// connection address. Note that the given request headers reqHeaders must be
// normalized with lower-cased keys for this function to work.
func setClientIPTags(span span, remoteAddr string, reqHeaders map[string][]string) {
	ipHeaders := defaultIPHeaders
	if len(clientIPHeader) > 0 {
		ipHeaders = []string{clientIPHeader}
	}

	var (
		headers []string
		ips     []string
	)
	for _, hdr := range ipHeaders {
		if v, _ := reqHeaders[hdr]; len(v) > 0 {
			headers = append(headers, hdr)
			ips = append(ips, v...)
		}
	}

	var remoteIP netip.Addr
	if remoteAddr != "" {
		remoteIP = parseIP(remoteAddr)
		if remoteIP.IsValid() {
			span.SetMetaTag("network.client.ip", remoteIP.String())
		}
	}

	switch len(ips) {
	case 0:
		ip := remoteIP.String()
		if remoteIP.IsValid() && isGlobal(remoteIP) {
			span.SetMetaTag("http.client_ip", ip)
		}
	case 1:
		for _, ipstr := range strings.Split(ips[0], ",") {
			ip := parseIP(strings.TrimSpace(ipstr))
			if ip.IsValid() && isGlobal(ip) {
				span.SetMetaTag("http.client_ip", ip.String())
				break
			}
		}
	default:
		for _, hdr := range headers {
			span.SetMetaTag("http.request.headers."+hdr, strings.Join(reqHeaders[hdr], ","))
		}
		span.SetMetaTag("_dd.multiple-ip-headers", strings.Join(headers, ","))
	}
}

func parseIP(s string) netip.Addr {
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		if ip, err := netip.ParseAddr(h); err == nil {
			return ip
		}
	}
	return netip.Addr{}
}

func isGlobal(ip netip.Addr) bool {
	// IsPrivate also checks for ipv6 ULA.
	// We care to check for these addresses are not considered public, hence not global.
	// See https://www.rfc-editor.org/rfc/rfc4193.txt for more details.
	isGlobal := !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
	if !isGlobal || !ip.Is6() {
		return isGlobal
	}
	for _, n := range ipv6SpecialNetworks {
		if n.Contains(ip) {
			return false
		}
	}
	return isGlobal
}
