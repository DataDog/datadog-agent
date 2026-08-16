// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"iter"
	"sort"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
)

const baselineSelectionsPerSnapshot = 3

type baselineCandidate struct {
	path       common.Pathtest
	pathHash   uint64
	diagnostic bool
	bytes      uint64
}

func (candidate baselineCandidate) betterThan(other baselineCandidate) bool {
	if candidate.diagnostic != other.diagnostic {
		return candidate.diagnostic
	}
	if candidate.bytes != other.bytes {
		return candidate.bytes > other.bytes
	}
	return candidate.pathHash < other.pathHash
}

// addBaselineCandidate keeps selected unique, best-first, and capped at three.
func addBaselineCandidate(selected []baselineCandidate, candidate baselineCandidate) []baselineCandidate {
	// A discarded observation cannot become a winner unless the same path is
	// observed later with a stronger score, at which point it is reconsidered.
	for i := range selected {
		if selected[i].pathHash != candidate.pathHash {
			continue
		}
		if !candidate.betterThan(selected[i]) {
			return selected
		}
		selected[i] = candidate
		sort.Slice(selected, func(i, j int) bool { return selected[i].betterThan(selected[j]) })
		return selected
	}

	if len(selected) < baselineSelectionsPerSnapshot {
		selected = append(selected, candidate)
	} else if candidate.betterThan(selected[len(selected)-1]) {
		selected[len(selected)-1] = candidate
	} else {
		return selected
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].betterThan(selected[j]) })
	return selected
}

func (s *npCollectorImpl) scheduleBaselineNetworkPathTests(conns iter.Seq[npmodel.NetworkPathConnection]) {
	vpcSubnets, err := s.getVPCSubnets()
	if err != nil {
		s.logger.Errorf("Failed to get VPC subnets to skip: %s", err)
		return
	}

	selected := make([]baselineCandidate, 0, baselineSelectionsPerSnapshot)
	for conn := range conns {
		evaluation := s.evaluateNetworkPathForConn(conn, payload.PathOriginNetworkTraffic, vpcSubnets)
		if !evaluation.shouldSchedule {
			continue
		}
		path := s.makePathtest(conn, payload.PathOriginNetworkTraffic)
		path.DynamicTestProfile = payload.DynamicTestProfileBaseline
		selected = addBaselineCandidate(selected, baselineCandidate{
			path:       path,
			pathHash:   path.GetHash(),
			diagnostic: conn.BaselineDiagnostic,
			bytes:      conn.BaselineBytes,
		})
	}

	for i := range selected {
		if err := s.scheduleOne(&selected[i].path); err != nil {
			s.logger.Errorf("Error scheduling baseline pathtest: %s", err)
		}
	}
}
