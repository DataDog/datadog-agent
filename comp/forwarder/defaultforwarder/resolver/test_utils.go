// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package resolver

import (
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// NewSingleDomainResolver creates a DomainResolver with its destination domain & API keys
func NewSingleDomainResolver(domain string, apiKeys []utils.APIKeys) (DomainResolver, error) {
	return NewSingleDomainResolver2(utils.EndpointDescriptor{BaseURL: domain, APIKeySet: apiKeys})
}
