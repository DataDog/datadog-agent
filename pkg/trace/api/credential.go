// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/credential"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

// authorizeEndpoint stamps the credential for endpoint e onto h. It returns true
// when the endpoint has a credential (either from a provider or a static API key)
// and false when the credential is not yet available, meaning the caller should
// skip this endpoint rather than send unauthenticated.
func authorizeEndpoint(e config.Endpoint, h http.Header) bool {
	return credential.StampAuth(h, e.CredentialProvider, e.APIKey)
}

// isDelaDirective reports whether a value is a DELA(...) directive.
func isDelaDirective(value string) bool {
	return credential.IsDirective(value)
}

// resolveCredentialProvider resolves the CredentialProvider for endpoint e when its
// API key is a DELA(...) directive. The directive text, configSettingPath, and host
// together identify the provider via AgentConfig.CredentialProviderFn.
func resolveCredentialProvider(conf *config.AgentConfig, e *config.Endpoint, apiKey, configSettingPath string) {
	if !isDelaDirective(apiKey) {
		return
	}
	e.CredentialDirective = apiKey
	e.APIKey = "" // clear the directive text so it can't leak as a literal key
	e.ConfigSettingPath = configSettingPath
	if conf.CredentialProviderFn != nil {
		e.CredentialProvider = conf.CredentialProviderFn(configSettingPath, e.Host, apiKey)
	}
}
