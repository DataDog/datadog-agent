// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import observer "github.com/DataDog/datadog-agent/comp/observer/def"

type incidentGraphBuilder interface {
	name() string
	supports(correlation observer.ActiveCorrelation) bool
	build(correlation observer.ActiveCorrelation) (IncidentGraph, error)
}

type rcaBuilderRegistry struct {
	builders []incidentGraphBuilder
}

func newRCABuilderRegistry(config RCAConfig) *rcaBuilderRegistry {
	return &rcaBuilderRegistry{
		builders: []incidentGraphBuilder{
			newTimeClusterRCABuilder(config),
		},
	}
}

func (r *rcaBuilderRegistry) build(correlation observer.ActiveCorrelation) (IncidentGraph, bool, error) {
	for _, b := range r.builders {
		if !b.supports(correlation) {
			continue
		}
		graph, err := b.build(correlation)
		return graph, true, err
	}
	return IncidentGraph{}, false, nil
}
