// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"fmt"

	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// misplacedConfig describes a dispatched config sitting on an unrestricted
// worker while eligible restricted workers are live, i.e. a check that fell
// back to the general pool during a runner group outage and was never moved
// back.
type misplacedConfig struct {
	digest   string
	srcNode  string
	destNode string
}

// repairMisplacedConfigs moves dispatched cluster check configs that
// previously fell back to unrestricted workers (e.g. while their dedicated
// runner group was entirely down) back onto an eligible restricted worker,
// now that one is live again.
//
// It runs only in mixed pools, where advanced dispatching is disabled: the
// presence of node agents turns the utilization rebalance off entirely, so
// without this pass a config that fell back to a node agent during a group
// outage would stay there indefinitely. (In runner-only pools the
// compatibility-aware utilization rebalance already re-places configs onto
// their group.)
//
// The pass is deliberately conservative and bounded:
//   - only configs whose current holder is unrestricted (no declared
//     compatibility) AND whose placement candidates include at least one
//     restricted worker are touched; every other config is left alone;
//   - the destination is the eligible restricted worker with the fewest
//     dispatched configs;
//   - it runs at most once per rebalance_period (10m by default), which is the
//     natural churn bound — each move is a check unschedule/schedule on the
//     worker, so we do not want this to follow a flapping group. A hysteresis
//     knob (e.g. requiring the group healthy for N minutes before reclaiming)
//     can be added later if group flapping turns out to be an issue.
func (d *dispatcher) repairMisplacedConfigs() {
	moves := d.collectMisplacedConfigs()

	for _, move := range moves {
		if err := d.repairMoveConfig(move.srcNode, move.destNode, move.digest); err != nil {
			log.Warnf("Cannot move back config %s from %s to %s: %v", move.digest, move.srcNode, move.destNode, err)
			continue
		}
		repairMoves.Inc(le.JoinLeaderValue)
		log.Infof("Moved config %s back onto its runner group: %s -> %s", move.digest, move.srcNode, move.destNode)
	}

	if len(moves) > 0 {
		log.Infof("Repair pass moved %d cluster check configs back onto their runner group", len(moves))
	}
}

// collectMisplacedConfigs returns the configs eligible for repair, with their
// chosen destination, without modifying the store.
func (d *dispatcher) collectMisplacedConfigs() []misplacedConfig {
	d.store.RLock()
	defer d.store.RUnlock()

	var moves []misplacedConfig

	for digest, nodeName := range d.store.digestToNode {
		config, found := d.store.digestToConfig[digest]
		if !found {
			continue
		}

		holder, found := d.store.nodes[nodeName]
		if !found {
			continue
		}

		holder.RLock()
		holderCompat := holder.checkCompat
		holder.RUnlock()
		if holderCompat != nil {
			// Already on a compat-declaring worker: not misplaced.
			continue
		}

		candidates := d.candidatesFromStore(config.Name)
		// candidates already applies the preference rule: when any restricted
		// worker is eligible, only restricted workers are returned. A
		// non-empty result therefore means a restricted worker can run the
		// check; an empty or unrestricted-only result means the group is still
		// down and the fallback placement is correct.
		if len(candidates) == 0 {
			continue
		}
		if c, found := d.store.nodes[candidates[0]]; found {
			c.RLock()
			restricted := c.checkCompat != nil
			c.RUnlock()
			if !restricted {
				continue
			}
		}

		dest := leastChecksNode(d.store.nodes, candidates)
		if dest == "" || dest == nodeName {
			continue
		}

		moves = append(moves, misplacedConfig{digest: digest, srcNode: nodeName, destNode: dest})
	}

	return moves
}

// leastChecksNode returns, among the given node names, the one with the
// fewest dispatched configs. Must be called with the store read-locked.
func leastChecksNode(nodes map[string]*nodeStore, names []string) string {
	selected := ""
	minNumChecks := 0

	for _, name := range names {
		node, found := nodes[name]
		if !found {
			continue
		}
		if selected == "" || len(node.digestToConfig) < minNumChecks {
			selected = name
			minNumChecks = len(node.digestToConfig)
		}
	}

	return selected
}

// repairMoveConfig reassigns a config from one node to another, without the
// runner-stats requirements of moveConfig: the source is typically a node
// agent, which does not expose CLC runner stats, so stats are moved on a
// best-effort basis only. Must not be called with the store locked.
func (d *dispatcher) repairMoveConfig(src, dest, digest string) error {
	if src == dest {
		return nil
	}

	log.Debugf("Repair-moving config %s from %s to %s", digest, src, dest)

	d.store.Lock()
	defer d.store.Unlock()

	destNode, destFound := d.store.getNodeStore(dest)
	sourceNode, srcFound := d.store.getNodeStore(src)
	config, configFound := d.store.digestToConfig[digest]
	if !destFound || !srcFound {
		return fmt.Errorf("node %s or %s not found", src, dest)
	}
	if !configFound {
		return fmt.Errorf("no config registered for digest %s", digest)
	}

	// Best-effort move of per-instance runner stats (usually absent on node agents).
	var instanceIDs []string
	for checkID, checkDigest := range d.store.idToDigest {
		if checkDigest == digest {
			instanceIDs = append(instanceIDs, string(checkID))
		}
	}
	for _, checkID := range instanceIDs {
		if stats, err := sourceNode.GetRunnerStats(checkID); err == nil {
			destNode.AddRunnerStats(checkID, stats)
			sourceNode.RemoveRunnerStats(checkID)
		}
	}

	// Reassign the config at the node level.
	d.store.digestToNode[digest] = dest

	sourceNode.Lock()
	sourceNode.removeConfig(digest)
	sourceNode.Unlock()

	destNode.Lock()
	destNode.addConfig(config)
	destNode.Unlock()

	// Re-key configsInfo from src to dest.
	for _, checkID := range instanceIDs {
		configsInfo.Delete(src, config.Name, checkID, le.JoinLeaderValue)
		configsInfo.Set(1.0, dest, config.Name, checkID, le.JoinLeaderValue)
	}

	log.Debugf("Config %s repair-moved from %s to %s", digest, src, dest)
	return nil
}
