// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(ASM) Fix revive linter
//nolint:deadcode,unused
package httpsec

import (
	"os"
	"sort"

	json "github.com/json-iterator/go"
)

// envClientIPHeader is the name of the env var used to specify the IP header to be used for client IP collection.
const envClientIPHeader = "DD_TRACE_CLIENT_IP_HEADER"

var (
	//nolint:unused // TODO(ASM) Fix unused linter
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

	// Configured list of IP-related headers leveraged to retrieve the public
	// client IP address. Defined at init-time in the init() function below.
	monitoredClientIPHeadersCfg []string

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
	if cfg := os.Getenv(envClientIPHeader); cfg != "" {
		// Collect this header value too
		collectedHTTPHeaders = append(collectedHTTPHeaders, cfg)
		// Set this IP header as the only one to consider for ClientIP()
		monitoredClientIPHeadersCfg = []string{cfg}
	} else {
		monitoredClientIPHeadersCfg = defaultIPHeaders
	}

	// Ensure the list of headers are sorted for sort.SearchStrings()
	sort.Strings(collectedHTTPHeaders[:])
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
	panic("not called")
}

// setEventSpanTags sets the security event span tags into the service entry span.
func setEventSpanTags(span span, events []any) error {
	panic("not called")
}

// Create the value of the security events tag.
func makeEventsTagValue(events []any) (json.RawMessage, error) {
	panic("not called")
}

// setSecurityEventsTags sets the AppSec-specific span tags when security events were found.
func setSecurityEventsTags(span span, events []any, headers, respHeaders map[string][]string) {
	panic("not called")
}

// setAPISecurityEventsTags sets the AppSec-specific span tags related to API security schemas
func setAPISecurityTags(span span, derivatives map[string]any) {
	panic("not called")
}

// normalizeHTTPHeaders returns the HTTP headers following Datadog's
// normalization format.
func normalizeHTTPHeaders(headers map[string][]string) (normalized map[string]string) {
	panic("not called")
}

// setClientIPTags sets the http.client_ip, http.request.headers.*, and
// network.client.ip span tags according to the request headers and remote
// connection address. Note that the given request headers reqHeaders must be
// normalized with lower-cased keys for this function to work.
func setClientIPTags(span span, remoteAddr string, reqHeaders map[string][]string) {
	panic("not called")
}
