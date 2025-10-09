// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package domainresolver

// IPToDomainResolver handle ip to domain resolution
type IPToDomainResolver struct {
	ipToDomainMap map[string]string
}

// NewIPToDomainResolver constructor
func NewIPToDomainResolver(ipToDomainMap map[string]string) *IPToDomainResolver {
	return &IPToDomainResolver{
		ipToDomainMap: ipToDomainMap,
	}
}

// ResolveIPToDomain returns a domain for an IP
func (r *IPToDomainResolver) ResolveIPToDomain(ip string) string {
	return r.ipToDomainMap[ip]
}
