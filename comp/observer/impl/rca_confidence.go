// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

func buildRCAConfidence(graph IncidentGraph, seriesCandidates []RCARootCandidate, config RCAConfig) RCAConfidence {
	config = config.normalized()

	dataLimited := len(graph.Nodes) < config.MinDataNodes || len(graph.Edges) == 0

	directedEdges := 0
	for _, edge := range graph.Edges {
		if edge.Directed {
			directedEdges++
		}
	}

	weakDirectionality := false
	if len(graph.Edges) > 0 {
		directedRatio := float64(directedEdges) / float64(len(graph.Edges))
		weakDirectionality = directedRatio < config.WeakDirectionalityThreshold
	}

	ambiguousRoots := false
	if len(seriesCandidates) >= 2 {
		gap := seriesCandidates[0].Score - seriesCandidates[1].Score
		ambiguousRoots = gap < config.AmbiguousRootMargin

		// Also flag ambiguity when the onset gap between top-2 candidates is
		// small relative to the cluster window span, even if scores differ.
		windowSpan := graph.ClusterWindowEnd - graph.ClusterWindowStart
		if windowSpan > 0 {
			onsetDelta := seriesCandidates[0].OnsetTime - seriesCandidates[1].OnsetTime
			if onsetDelta < 0 {
				onsetDelta = -onsetDelta
			}
			onsetGapRatio := float64(onsetDelta) / float64(windowSpan)
			if onsetGapRatio < 0.1 {
				ambiguousRoots = true
			}
		}
	}

	baseScore := 0.3
	if len(seriesCandidates) > 0 {
		baseScore = clamp01(seriesCandidates[0].Score)
	}

	penalty := 0.0
	if dataLimited {
		penalty += 0.20
	}
	if weakDirectionality {
		penalty += 0.20
	}
	if ambiguousRoots {
		penalty += 0.20
	}

	return RCAConfidence{
		Score:              roundScore(clamp01(baseScore - penalty)),
		DataLimited:        dataLimited,
		WeakDirectionality: weakDirectionality,
		AmbiguousRoots:     ambiguousRoots,
	}
}
