// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"strings"
)

func extractEvidencePaths(graph IncidentGraph, seriesCandidates []RCARootCandidate, config RCAConfig) []RCAEvidencePath {
	if len(graph.Nodes) == 0 || len(seriesCandidates) == 0 {
		return nil
	}
	config = config.normalized()

	nodeByID := make(map[string]IncidentNode, len(graph.Nodes))
	maxPersistence := 1
	for _, node := range graph.Nodes {
		nodeByID[node.SeriesID] = node
		if node.PersistenceCount > maxPersistence {
			maxPersistence = node.PersistenceCount
		}
	}

	adjacency := make(map[string][]string)
	for _, edge := range graph.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		if !edge.Directed {
			adjacency[edge.To] = append(adjacency[edge.To], edge.From)
		}
	}
	for nodeID := range adjacency {
		sort.Strings(adjacency[nodeID])
		adjacency[nodeID] = dedupStrings(adjacency[nodeID])
	}

	nodeSalience := make(map[string]float64, len(graph.Nodes))
	for _, node := range graph.Nodes {
		salience := node.PeakScore + (float64(node.PersistenceCount) / float64(maxPersistence))
		nodeSalience[node.SeriesID] = salience
	}

	seen := make(map[string]struct{})
	paths := make([]RCAEvidencePath, 0, config.MaxEvidencePaths)

	for _, root := range seriesCandidates {
		if len(paths) >= config.MaxEvidencePaths {
			break
		}

		if _, ok := nodeByID[root.ID]; !ok {
			continue
		}

		dist, prev := shortestPathsFrom(root.ID, adjacency)
		bestTarget := ""
		bestTargetScore := -1.0
		for nodeID, d := range dist {
			if nodeID == root.ID || d <= 0 {
				continue
			}
			targetNode := nodeByID[nodeID]
			rootNode := nodeByID[root.ID]
			laterBonus := 0.0
			if targetNode.OnsetTime >= rootNode.OnsetTime {
				laterBonus = 0.2
			}
			candidate := nodeSalience[nodeID] + laterBonus
			if candidate > bestTargetScore || (candidate == bestTargetScore && nodeID < bestTarget) {
				bestTargetScore = candidate
				bestTarget = nodeID
			}
		}

		var nodePath []string
		if bestTarget == "" {
			nodePath = []string{root.ID}
		} else {
			nodePath = reconstructPath(root.ID, bestTarget, prev)
		}
		if len(nodePath) == 0 {
			continue
		}

		sig := strings.Join(nodePath, "->")
		if _, exists := seen[sig]; exists {
			continue
		}
		seen[sig] = struct{}{}

		pathScore := clamp01(root.Score)
		if bestTarget != "" {
			pathScore = clamp01(0.6*root.Score + 0.4*normalizeSalience(bestTargetScore))
		}

		path := RCAEvidencePath{
			Nodes: nodePath,
			Score: roundScore(pathScore),
		}
		if len(nodePath) > 1 {
			path.Why = fmt.Sprintf("temporal path from likely root to symptom across %d hop(s)", len(nodePath)-1)
		} else {
			path.Why = "top-ranked root candidate with limited downstream evidence"
		}

		paths = append(paths, path)
	}

	if len(paths) == 0 {
		paths = append(paths, RCAEvidencePath{
			Nodes: []string{seriesCandidates[0].ID},
			Score: roundScore(clamp01(seriesCandidates[0].Score)),
			Why:   "insufficient graph connectivity for multi-node path",
		})
	}

	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Score != paths[j].Score {
			return paths[i].Score > paths[j].Score
		}
		if len(paths[i].Nodes) != len(paths[j].Nodes) {
			return len(paths[i].Nodes) > len(paths[j].Nodes)
		}
		return strings.Join(paths[i].Nodes, "|") < strings.Join(paths[j].Nodes, "|")
	})

	if len(paths) > config.MaxEvidencePaths {
		paths = paths[:config.MaxEvidencePaths]
	}

	return paths
}

func shortestPathsFrom(root string, adjacency map[string][]string) (map[string]int, map[string]string) {
	dist := map[string]int{root: 0}
	prev := make(map[string]string)
	queue := []string{root}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, next := range adjacency[current] {
			if _, seen := dist[next]; seen {
				continue
			}
			dist[next] = dist[current] + 1
			prev[next] = current
			queue = append(queue, next)
		}
	}

	return dist, prev
}

func reconstructPath(start, target string, prev map[string]string) []string {
	if start == target {
		return []string{start}
	}

	rev := []string{target}
	cur := target
	for cur != start {
		p, ok := prev[cur]
		if !ok {
			return nil
		}
		rev = append(rev, p)
		cur = p
	}

	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

func dedupStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := items[:1]
	for i := 1; i < len(items); i++ {
		if items[i] == items[i-1] {
			continue
		}
		out = append(out, items[i])
	}
	return out
}

func normalizeSalience(v float64) float64 {
	if v <= 0 {
		return 0
	}
	return v / (1 + v)
}
