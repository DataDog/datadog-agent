// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"math"
	"sort"
)

type temporalNodeScore struct {
	Node              IncidentNode
	Score             float64
	OnsetFactor       float64
	CoverageFactor    float64
	PersistenceFactor float64
	SeverityFactor    float64
	IncomingPenalty   float64
	SpreadPenalty     float64
}

func rankSeriesRootCandidates(graph IncidentGraph, config RCAConfig) []RCARootCandidate {
	if len(graph.Nodes) == 0 {
		return nil
	}
	config = config.normalized()

	scored := scoreTemporalNodes(graph, config)
	if len(scored) == 0 {
		return nil
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		if scored[i].Node.OnsetTime != scored[j].Node.OnsetTime {
			return scored[i].Node.OnsetTime < scored[j].Node.OnsetTime
		}
		return scored[i].Node.SeriesID < scored[j].Node.SeriesID
	})

	limit := config.MaxRootCandidates
	if limit > len(scored) {
		limit = len(scored)
	}

	results := make([]RCARootCandidate, 0, limit)
	for _, nodeScore := range scored[:limit] {
		reasons := make([]string, 0, 6)
		if nodeScore.OnsetFactor >= 0.7 {
			reasons = append(reasons, "early onset compared with peer anomalies")
		}
		if nodeScore.CoverageFactor > 0 {
			reasons = append(reasons, fmt.Sprintf("downstream temporal coverage %.2f", nodeScore.CoverageFactor))
		}
		if nodeScore.PersistenceFactor > 0.5 {
			reasons = append(reasons, fmt.Sprintf("persistence %.2f", nodeScore.PersistenceFactor))
		}
		if nodeScore.SeverityFactor >= 0.7 {
			reasons = append(reasons, fmt.Sprintf("high severity (peak sigma %.2f)", nodeScore.SeverityFactor))
		}
		if nodeScore.IncomingPenalty >= 0.6 {
			reasons = append(reasons, "strong incoming support from earlier nodes reduces root confidence")
		}
		if nodeScore.SpreadPenalty > 0 {
			reasons = append(reasons, fmt.Sprintf("namespace underrepresented in cluster (spread penalty %.2f)", nodeScore.SpreadPenalty))
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "supported by temporal ordering and cluster structure")
		}

		results = append(results, RCARootCandidate{
			ID:        nodeScore.Node.SeriesID,
			Score:     roundScore(nodeScore.Score),
			OnsetTime: nodeScore.Node.OnsetTime,
			Why:       reasons,
		})
	}

	return results
}

func scoreTemporalNodes(graph IncidentGraph, config RCAConfig) []temporalNodeScore {
	nodeCount := len(graph.Nodes)
	if nodeCount == 0 {
		return nil
	}

	earliest, latest := nodeOnsetBounds(graph.Nodes)
	maxPersistence := 1
	maxPeakScore := 0.0
	namespaceCounts := make(map[string]int)
	nodeNamespace := make(map[string]string)
	for _, n := range graph.Nodes {
		if n.PersistenceCount > maxPersistence {
			maxPersistence = n.PersistenceCount
		}
		if n.PeakScore > maxPeakScore {
			maxPeakScore = n.PeakScore
		}
		ns, _, _, ok := parseSeriesKey(n.SeriesID)
		if ok && ns != "" {
			namespaceCounts[ns]++
			nodeNamespace[n.SeriesID] = ns
		}
	}
	numNamespaces := len(namespaceCounts)
	totalNodes := len(graph.Nodes)

	outgoing := make(map[string][]string)
	incomingWeight := make(map[string]float64)
	maxIncoming := 0.0

	for _, edge := range graph.Edges {
		if edge.Directed {
			outgoing[edge.From] = append(outgoing[edge.From], edge.To)
			incomingWeight[edge.To] += edge.Weight
			if incomingWeight[edge.To] > maxIncoming {
				maxIncoming = incomingWeight[edge.To]
			}
		}
	}

	for nodeID := range outgoing {
		sort.Strings(outgoing[nodeID])
	}

	scores := make([]temporalNodeScore, 0, nodeCount)
	for _, node := range graph.Nodes {
		onsetFactor := 1.0
		if latest > earliest {
			onsetFactor = float64(latest-node.OnsetTime) / float64(latest-earliest)
		}
		if onsetFactor < 0 {
			onsetFactor = 0
		}

		coverageFactor := 0.0
		if nodeCount > 1 {
			reachable := reachableCount(node.SeriesID, outgoing)
			coverageFactor = float64(reachable) / float64(nodeCount-1)
		}

		persistenceFactor := float64(node.PersistenceCount) / float64(maxPersistence)

		severityFactor := 0.0
		if maxPeakScore > 0 {
			severityFactor = node.PeakScore / maxPeakScore
		}

		incomingPenalty := 0.0
		if maxIncoming > 0 {
			incomingPenalty = incomingWeight[node.SeriesID] / maxIncoming
		}

		spreadPenalty := 0.0
		if numNamespaces >= 2 {
			if ns, ok := nodeNamespace[node.SeriesID]; ok {
				expectedShare := 1.0 / float64(numNamespaces)
				actualShare := float64(namespaceCounts[ns]) / float64(totalNodes)
				if actualShare < expectedShare {
					spreadPenalty = 1.0 - (actualShare / expectedShare)
				}
			}
		}

		score := config.Weights.Onset*onsetFactor +
			config.Weights.DownstreamCoverage*coverageFactor +
			config.Weights.Persistence*persistenceFactor +
			config.Weights.Severity*severityFactor -
			config.Weights.IncomingPenalty*incomingPenalty -
			config.Weights.SpreadPenalty*spreadPenalty
		score = clamp01(score)

		scores = append(scores, temporalNodeScore{
			Node:              node,
			Score:             score,
			OnsetFactor:       onsetFactor,
			CoverageFactor:    coverageFactor,
			PersistenceFactor: persistenceFactor,
			SeverityFactor:    severityFactor,
			IncomingPenalty:   incomingPenalty,
			SpreadPenalty:     spreadPenalty,
		})
	}

	return scores
}

func reachableCount(root string, adjacency map[string][]string) int {
	visited := map[string]struct{}{root: {}}
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if _, seen := visited[next]; seen {
				continue
			}
			visited[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	if len(visited) == 0 {
		return 0
	}
	return len(visited) - 1
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundScore(v float64) float64 {
	return math.Round(v*1000) / 1000
}
