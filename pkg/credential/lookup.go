// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package credential

// Lookup resolves the credential provider for one additional endpoint,
// identified by the config setting it came from, its host, and its DELA(...)
// directive text. It returns nil when that endpoint has no delegated-auth
// instance (e.g. a plain API key or an unsupported cloud provider).
//
// This is the shared type for the "CredentialProviderFn" / "CredentialProviderLookup"
// pattern that was previously declared separately in pkg/trace/config and
// comp/logs/agent/config.
type Lookup func(configKey, host, directive string) Provider
