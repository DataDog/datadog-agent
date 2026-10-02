// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package identity

import (
	"errors"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

// connectionsDNS rewrites the lookup keys before endpoint addresses change.
// A source address can occur in both local and remote scopes; each needs a
// separate rewritten lookup while domain identities stay shared within a run.
func (m *Map) connectionsDNS(connections *model.CollectorConnections) (result error) {
	defer func() {
		if recover() != nil {
			result = errors.New("cannot rewrite captured connection DNS")
		}
	}()
	if len(connections.EncodedDNS) == 0 && len(connections.EncodedDnsLookups) == 0 {
		return nil
	}
	invalid := errors.New("cannot rewrite captured connection DNS")
	if len(connections.EncodedDNS) == 0 && len(connections.EncodedDomainDatabase) == 0 {
		return invalid
	}
	if len(connections.EncodedDNS) == 0 && telemetry.ValidateConnectionDNSV2(connections.EncodedDomainDatabase, connections.EncodedDnsLookups) != nil {
		return invalid
	}
	dnsSource := *connections
	if len(dnsSource.EncodedDNS) == 0 {
		dnsSource.EncodedDNS = nil
	}
	// A captured address can legitimately expand into local and remote scopes.
	const maxDNSItems = 2 * 65536
	type endpointKey struct{ address, scope string }
	seen := make(map[endpointKey]bool)
	domainIndices := make(map[string]int32)
	var domains []string
	lookups := make(map[string]*model.DNSDatabaseEntry)
	associations := 0
	add := func(address *model.Addr, scope string) error {
		if address == nil || address.Ip == "" {
			return nil
		}
		key := endpointKey{address.Ip, scope}
		if seen[key] {
			return nil
		}
		if len(seen) == maxDNSItems {
			return invalid
		}
		seen[key] = true
		entry := &model.DNSDatabaseEntry{}
		failed := false
		err := dnsSource.IterateDNS(address, func(_, total int, name string) bool {
			if name == "" || total > maxDNSItems || associations == maxDNSItems {
				failed = true
				return false
			}
			associations++
			name = m.replaceTokens(name)
			index, exists := domainIndices[name]
			if !exists {
				index = int32(len(domains))
				domainIndices[name] = index
				domains = append(domains, name)
			}
			entry.NameOffsets = append(entry.NameOffsets, index)
			return true
		})
		if err != nil || failed {
			return invalid
		}
		if len(entry.NameOffsets) != 0 {
			address := ip(scope, address.Ip)
			if existing := lookups[address]; existing != nil {
				existing.NameOffsets = append(existing.NameOffsets, entry.NameOffsets...)
			} else {
				lookups[address] = entry
			}
		}
		return nil
	}
	for _, connection := range connections.Connections {
		if connection == nil {
			continue
		}
		if err := add(connection.Laddr, m.scope); err != nil {
			return err
		}
		scope := m.runID
		if connection.IntraHost {
			scope = m.scope
		}
		if err := add(connection.Raddr, scope); err != nil {
			return err
		}
	}
	encoder := model.NewV2DNSEncoder()
	encoded, offsets, err := encoder.EncodeDomainDatabase(domains)
	if err != nil {
		return invalid
	}
	lookupsEncoded, err := encoder.EncodeMapped(lookups, offsets)
	if err != nil {
		return invalid
	}
	connections.EncodedDNS, connections.Domains = nil, nil
	connections.EncodedDomainDatabase, connections.EncodedDnsLookups = encoded, lookupsEncoded
	return nil
}
