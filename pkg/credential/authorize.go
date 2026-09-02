// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package credential

import "net/http"

// apiKeyHeader is the header a resolved credential is stamped onto. It lives
// here, inside StampAuth, rather than at each consumer's send site. When
// delegated auth moves from API keys to stateless tokens, the header name and
// credential format change here and nowhere else.
const apiKeyHeader = "DD-Api-Key"

// StampAuth stamps the credential onto h. It tries the provider first, then
// falls back to the static key. A provider that is still resolving returns
// false. Without a provider, it preserves the existing static path, including
// its empty-key behavior.
//
// This is the shared implementation of the "provider-or-static-key" pattern
// that was previously duplicated across the resolver, logs, trace writer, and
// trace API.
func StampAuth(h http.Header, provider Provider, staticKey string) bool {
	if provider != nil {
		return provider.Authorize(h)
	}
	h.Set(apiKeyHeader, staticKey)
	return true
}
