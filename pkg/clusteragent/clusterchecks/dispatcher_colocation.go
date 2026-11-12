// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// colocationRule is implemented by helmReleasesColocation, letting the
// dispatcher glue below keep configs together without knowing which checks
// they belong to.
type colocationRule interface {
	isEnabled() bool
	// group returns the colocation group of config, or "" if it has none.
	group(config integration.Config) string
}

// colocationGroup returns the colocation group of a config, or "" if it has
// none. Configs of the same group are dispatched to the same runner.
func (d *dispatcher) colocationGroup(config integration.Config) string {
	for _, rule := range d.colocationRules {
		if !rule.isEnabled() {
			continue
		}
		if group := rule.group(config); group != "" {
			return group
		}
	}
	return ""
}

// colocatedNode returns a node already running a config of the same
// colocation group as config that may run config too, or "" when there is none.
//
// The store must be read-locked.
func (d *dispatcher) colocatedNode(config integration.Config) string {
	group := d.colocationGroup(config)
	if group == "" {
		return ""
	}

	digest := config.Digest()
	for otherDigest, nodeName := range d.store.digestToNode {
		if otherDigest == digest {
			continue
		}
		other, found := d.store.digestToConfig[otherDigest]
		if !found || d.colocationGroup(other) != group {
			continue
		}

		node, found := d.store.nodes[nodeName]
		if !found {
			continue
		}
		if !d.accepts(node, config.Name) {
			log.Debugf("Cannot colocate %s with %s on node %s for group %q: the node doesn't accept it", config.Name, other.Name, nodeName, group)
			continue
		}
		return nodeName
	}
	return ""
}

// hasColocatedPeer reports whether another config of the colocation group of
// config runs on nodeName, so config must stay there. The store must be read-locked.
func (d *dispatcher) hasColocatedPeer(digest string, config integration.Config, nodeName string) bool {
	group := d.colocationGroup(config)
	if group == "" {
		return false
	}
	for otherDigest, otherNode := range d.store.digestToNode {
		if otherDigest == digest || otherNode != nodeName {
			continue
		}
		if other, found := d.store.digestToConfig[otherDigest]; found && d.colocationGroup(other) == group {
			return true
		}
	}
	return false
}

// getNodeToSchedule returns the node where a new config should be scheduled:
// the node of its colocation group when it has one, otherwise the node chosen
// for its check. The store must be read-locked.
func (d *dispatcher) getNodeToSchedule(config integration.Config) (node string, anyNode bool) {
	if node = d.colocatedNode(config); node != "" {
		return node, true
	}
	return d.getNodeToScheduleCheck(config.Name)
}
