// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"math"
	"sort"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

type timeClusterRCABuilder struct {
	config RCAConfig
}

func newTimeClusterRCABuilder(config RCAConfig) *timeClusterRCABuilder {
	return &timeClusterRCABuilder{config: config.normalized()}
}

func (b *timeClusterRCABuilder) name() string {
	return "time_cluster_builder"
}

func (b *timeClusterRCABuilder) supports(correlation observer.ActiveCorrelation) bool {
	if b.config.Correlator != "" && b.config.Correlator != "time_cluster" {
		return false
	}
	if strings.HasPrefix(correlation.Pattern, "time_cluster_") {
		return true
	}
	return strings.Contains(strings.ToLower(correlation.Title), "timecluster")
}

func (b *timeClusterRCABuilder) build(correlation observer.ActiveCorrelation) (IncidentGraph, error) {
	type nodeAggregate struct {
		seriesID   string
		metricName string
		onsetTime  int64
		peakScore  float64
		tags       []string
		count      int
	}

	nodeBySeries := make(map[string]*nodeAggregate)

	for _, anomaly := range correlation.Anomalies {
		seriesID := string(anomaly.SourceSeriesID)
		if seriesID == "" {
			seriesID = string(anomaly.Source)
		}
		if seriesID == "" {
			continue
		}

		metricName := string(anomaly.Source)
		if metricName == "" {
			metricName = metricNameFromSeriesID(seriesID)
		}

		ts := anomalyTime(anomaly)
		peak := anomalyPeakScore(anomaly)

		agg, found := nodeBySeries[seriesID]
		if !found {
			nodeBySeries[seriesID] = &nodeAggregate{
				seriesID:   seriesID,
				metricName: metricName,
				onsetTime:  ts,
				peakScore:  peak,
				tags:       append([]string(nil), anomaly.Tags...),
				count:      1,
			}
			continue
		}

		agg.count++
		if ts > 0 && (agg.onsetTime == 0 || ts < agg.onsetTime) {
			agg.onsetTime = ts
		}
		if peak > agg.peakScore {
			agg.peakScore = peak
		}
		if len(agg.tags) == 0 && len(anomaly.Tags) > 0 {
			agg.tags = append([]string(nil), anomaly.Tags...)
		}
		if agg.metricName == "" && metricName != "" {
			agg.metricName = metricName
		}
	}

	for _, seriesID := range correlation.MemberSeriesIDs {
		key := string(seriesID)
		if key == "" {
			continue
		}
		if _, found := nodeBySeries[key]; found {
			continue
		}
		nodeBySeries[key] = &nodeAggregate{
			seriesID:   key,
			metricName: metricNameFromSeriesID(key),
			onsetTime:  correlation.FirstSeen,
			peakScore:  1.0,
			count:      1,
		}
	}

	nodes := make([]IncidentNode, 0, len(nodeBySeries))
	for _, agg := range nodeBySeries {
		node := IncidentNode{
			SeriesID:         agg.seriesID,
			MetricName:       agg.metricName,
			OnsetTime:        agg.onsetTime,
			PersistenceCount: agg.count,
			PeakScore:        agg.peakScore,
			Tags:             append([]string(nil), agg.tags...),
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].SeriesID < nodes[j].SeriesID })

	if len(nodes) == 0 {
		return IncidentGraph{ClusterID: correlation.Pattern}, nil
	}

	windowStart := correlation.FirstSeen
	windowEnd := correlation.LastUpdated
	minOnset, maxOnset := nodeOnsetBounds(nodes)
	if windowStart == 0 || (minOnset > 0 && minOnset < windowStart) {
		windowStart = minOnset
	}
	if windowEnd == 0 || maxOnset > windowEnd {
		windowEnd = maxOnset
	}

	edges := buildTemporalProximityEdges(nodes, b.config)

	return IncidentGraph{
		ClusterID:          correlation.Pattern,
		ClusterWindowStart: windowStart,
		ClusterWindowEnd:   windowEnd,
		Nodes:              nodes,
		Edges:              edges,
	}, nil
}

func metricNameFromSeriesID(seriesID string) string {
	parts := strings.SplitN(seriesID, "|", 3)
	if len(parts) < 2 {
		return seriesID
	}
	if parts[1] == "" {
		return seriesID
	}
	return parts[1]
}

func anomalyTime(anomaly observer.AnomalyOutput) int64 {
	if anomaly.TimeRange.End > 0 {
		return anomaly.TimeRange.End
	}
	if anomaly.Timestamp > 0 {
		return anomaly.Timestamp
	}
	if anomaly.TimeRange.Start > 0 {
		return anomaly.TimeRange.Start
	}
	return 0
}

func anomalyPeakScore(anomaly observer.AnomalyOutput) float64 {
	if anomaly.DebugInfo != nil {
		sigma := math.Abs(anomaly.DebugInfo.DeviationSigma)
		if sigma > 0 {
			return sigma
		}
	}
	return 1.0
}

func nodeOnsetBounds(nodes []IncidentNode) (int64, int64) {
	if len(nodes) == 0 {
		return 0, 0
	}
	minOnset := nodes[0].OnsetTime
	maxOnset := nodes[0].OnsetTime
	for _, n := range nodes[1:] {
		if minOnset == 0 || (n.OnsetTime > 0 && n.OnsetTime < minOnset) {
			minOnset = n.OnsetTime
		}
		if n.OnsetTime > maxOnset {
			maxOnset = n.OnsetTime
		}
	}
	return minOnset, maxOnset
}

func buildTemporalProximityEdges(nodes []IncidentNode, config RCAConfig) []IncidentEdge {
	epsilon := config.OnsetEpsilonSeconds
	maxLag := config.MaxEdgeLagSeconds
	if maxLag <= 0 {
		maxLag = DefaultRCAConfig().MaxEdgeLagSeconds
	}

	edges := make([]IncidentEdge, 0, len(nodes)*2)
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			left := nodes[i]
			right := nodes[j]
			lag := right.OnsetTime - left.OnsetTime
			absLag := lag
			if absLag < 0 {
				absLag = -absLag
			}
			if maxLag > 0 && absLag > maxLag {
				continue
			}

			weight := 1.0 / (1.0 + float64(absLag))

			if absLag <= epsilon {
				from, to := left.SeriesID, right.SeriesID
				if to < from {
					from, to = to, from
				}
				edges = append(edges, IncidentEdge{
					From:       from,
					To:         to,
					EdgeType:   edgeTypeTimeProximity,
					Weight:     weight,
					LagSeconds: int64Ptr(absLag),
					Directed:   false,
				})
				continue
			}

			fromNode := left
			toNode := right
			signedLag := lag
			if lag < 0 {
				fromNode = right
				toNode = left
				signedLag = -lag
			}

			edges = append(edges, IncidentEdge{
				From:       fromNode.SeriesID,
				To:         toNode.SeriesID,
				EdgeType:   edgeTypeTimeProximity,
				Weight:     weight,
				LagSeconds: int64Ptr(signedLag),
				Directed:   true,
			})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		if edges[i].Directed != edges[j].Directed {
			return edges[i].Directed
		}
		return edges[i].Weight > edges[j].Weight
	})

	return edges
}

func int64Ptr(v int64) *int64 {
	value := v
	return &value
}
