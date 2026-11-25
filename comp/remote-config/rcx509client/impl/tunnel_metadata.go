// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcx509clientimpl

import (
	"time"

	inventorychecks "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// tunnelConnectionIDKey is the check-instance inventory metadata key that
	// carries the current RC connection ID. The key's presence means that the
	// instance's Agent has a registered Remote Queries Execute handler on that
	// connection. Anyone holding the org's API key can write inventory metadata,
	// so the value is a routing hint, never an authentication.
	tunnelConnectionIDKey = "remote_queries.tunnel_connection_id"

	// tunnelMetadataInterval is how often the ID is applied again, so that
	// instances scheduled or rescheduled after a connection get the key.
	// Re-applying an unchanged value is a no-op in inventorychecks.
	tunnelMetadataInterval = 30 * time.Second
)

// tunnelMetadataPublisher publishes the connection ID in the inventory
// metadata of every loaded check instance that can serve a remote query: the
// same set the Remote Queries Service resolves against (check.RemoteQuerier
// with SupportsRemoteQueries true, which for Python checks means a callable
// remote_query).
type tunnelMetadataPublisher struct {
	inventory inventorychecks.Component
	checks    func() []check.Check
}

func newTunnelMetadataPublisher(inventory inventorychecks.Component, checks func() []check.Check) *tunnelMetadataPublisher {
	if inventory == nil || checks == nil {
		return nil
	}
	return &tunnelMetadataPublisher{inventory: inventory, checks: checks}
}

func (p *tunnelMetadataPublisher) publish(connectionID string) {
	for _, c := range p.checks() {
		if querier, ok := c.(check.RemoteQuerier); ok && querier.SupportsRemoteQueries() {
			p.inventory.Set(string(c.ID()), tunnelConnectionIDKey, connectionID)
		}
	}
}
