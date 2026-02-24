// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import "sort"

func rollupMetricRootCandidates(graph IncidentGraph, seriesCandidates []RCARootCandidate, maxCandidates int) []RCARootCandidate {
	if len(seriesCandidates) == 0 {
		return nil
	}

	nodeBySeries := make(map[string]IncidentNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodeBySeries[node.SeriesID] = node
	}

	type metricAggregate struct {
		scoreProduct float64
		onsetTime    int64
		count        int
	}

	agg := make(map[string]*metricAggregate)
	for _, series := range seriesCandidates {
		node, ok := nodeBySeries[series.ID]
		metricID := series.ID
		if ok && node.MetricName != "" {
			metricID = node.MetricName
		}
		if metricID == "" {
			continue
		}

		metricAgg, found := agg[metricID]
		if !found {
			agg[metricID] = &metricAggregate{
				scoreProduct: 1 - clamp01(series.Score),
				onsetTime:    series.OnsetTime,
				count:        1,
			}
			continue
		}

		metricAgg.scoreProduct *= (1 - clamp01(series.Score))
		if metricAgg.onsetTime == 0 || (series.OnsetTime > 0 && series.OnsetTime < metricAgg.onsetTime) {
			metricAgg.onsetTime = series.OnsetTime
		}
		metricAgg.count++
	}

	results := make([]RCARootCandidate, 0, len(agg))
	for metricID, metricAgg := range agg {
		metricScore := 1 - metricAgg.scoreProduct
		results = append(results, RCARootCandidate{
			ID:        metricID,
			Score:     roundScore(clamp01(metricScore)),
			OnsetTime: metricAgg.onsetTime,
			Why: []string{
				"probabilistic rollup of top series candidates",
			},
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].OnsetTime != results[j].OnsetTime {
			return results[i].OnsetTime < results[j].OnsetTime
		}
		return results[i].ID < results[j].ID
	})

	if maxCandidates > 0 && len(results) > maxCandidates {
		results = results[:maxCandidates]
	}

	return results
}
