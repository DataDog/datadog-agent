// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package status

// SourceToTrack represents a source tracker for a given integration space
type SourceToTrack struct {
	integrationName string
	tracker         *Tracker
}

// NewSourceToTrack returns a new SourceToTrack
func NewSourceToTrack(integrationName string, tracker *Tracker) *SourceToTrack {
	return &SourceToTrack{
		integrationName: integrationName,
		tracker:         tracker,
	}
}

// Builder builds the status
type Builder struct {
	integrationsTrackers map[string][]*Tracker
}

// NewBuilder returns a new Builder
func NewBuilder(sourcesToTrack []*SourceToTrack) *Builder {
	integrationsTrackers := make(map[string][]*Tracker)
	for _, source := range sourcesToTrack {
		_, exists := integrationsTrackers[source.integrationName]
		if !exists {
			integrationsTrackers[source.integrationName] = []*Tracker{}
		}
		integrationsTrackers[source.integrationName] = append(integrationsTrackers[source.integrationName], source.tracker)
	}
	return &Builder{
		integrationsTrackers: integrationsTrackers,
	}
}

// Build returns the status
func (b *Builder) Build() Status {
	integrations := []Integration{}
	for name, trackers := range b.integrationsTrackers {
		sources := []Source{}
		for _, tracker := range trackers {
			sources = append(sources, tracker.GetSource())
		}
		integrations = append(integrations, Integration{Name: name, Sources: sources})
	}
	return Status{
		IsRunning:    true,
		Integrations: integrations,
	}
}
