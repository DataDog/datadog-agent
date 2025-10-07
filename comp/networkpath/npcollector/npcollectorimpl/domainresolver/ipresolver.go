// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package domainresolver

// IpToDomainResolver handle ip to domain resolution
type IpToDomainResolver struct {
	ipToDomainMap map[string]string
}

// NewIpToDomainResolver constructor
func NewIpToDomainResolver(ipToDomainMap map[string]string) *IpToDomainResolver {
	return &IpToDomainResolver{
		ipToDomainMap: ipToDomainMap,
	}
}

// ResolveIPToDomain returns a domain for an IP
func (r *IpToDomainResolver) ResolveIPToDomain(ip string) string {
	return r.ipToDomainMap[ip]
}
