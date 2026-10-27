// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package routing holds injected log delivery group definitions and source progress.
// It does not load a routing policy or activate routing in the Agent.
package routing

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// Group is an independent delivery obligation. Extra endpoints retain the normal
// sender's copy semantics within this group. Stream declares groups that receive
// exactly the same records and may share a batch and its compressed bytes.
type Group struct {
	ID        string
	Endpoints *config.Endpoints
	Stream    string
}

// Selection is a resolved, explicit set of recipients for one source record.
// Excluded is an intentional local exclusion; it must never accompany recipients.
// Generation changes when a source is replaced under the same checkpoint identifier.
type Selection struct {
	Groups     []string
	Excluded   bool
	Generation uint64
}

// Definitions validates the complete injected registry before any workers start.
type Definitions struct {
	groups  map[string]Group
	streams map[string][]string
	order   []string
}

// MaxEndpoints caps the aggregate number of destination queues and HTTP pools
// created by an injected routed provider, including optional extra endpoints.
const MaxEndpoints = 32

var (
	// ErrInvalidSelection means no recipient may be inferred or acknowledged.
	ErrInvalidSelection = errors.New("invalid log routing selection")
	// ErrInvalidDefinition means no workers may be started for this registry.
	ErrInvalidDefinition = errors.New("invalid log routing definition")
)

// NewDefinitions validates the registry and the configured group limit.
func NewDefinitions(groups []Group, maxGroups int) (*Definitions, error) {
	if maxGroups < 1 || len(groups) == 0 || len(groups) > maxGroups {
		return nil, fmt.Errorf("%w: %d groups with limit %d", ErrInvalidDefinition, len(groups), maxGroups)
	}
	d := &Definitions{groups: make(map[string]Group, len(groups)), streams: make(map[string][]string)}
	totalEndpoints := 0
	for _, group := range groups {
		if group.ID == "" || group.Endpoints == nil || !group.Endpoints.UseHTTP || group.Endpoints.UseProto || !group.Endpoints.Main.IsReliable() || len(group.Endpoints.Endpoints) == 0 || group.Endpoints.BatchWait <= 0 || group.Endpoints.BatchMaxSize <= 0 || group.Endpoints.BatchMaxContentSize <= 0 || group.Endpoints.BatchMaxConcurrentSend < 0 {
			return nil, fmt.Errorf("%w: group %q needs a reliable HTTP primary", ErrInvalidDefinition, group.ID)
		}
		primary := group.Endpoints.Endpoints[0]
		if !primary.IsReliable() || primary.Host != group.Endpoints.Main.Host || primary.Port != group.Endpoints.Main.Port || primary.GetAPIKey() != group.Endpoints.Main.GetAPIKey() {
			return nil, fmt.Errorf("%w: group %q has no matching primary destination", ErrInvalidDefinition, group.ID)
		}
		totalEndpoints += len(group.Endpoints.Endpoints)
		if totalEndpoints > MaxEndpoints {
			return nil, fmt.Errorf("%w: more than %d total endpoints", ErrInvalidDefinition, MaxEndpoints)
		}
		if _, exists := d.groups[group.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate group %q", ErrInvalidDefinition, group.ID)
		}
		if group.Endpoints.Main.IsMRF {
			return nil, fmt.Errorf("%w: group %q uses MRF", ErrInvalidDefinition, group.ID)
		}
		for _, extra := range group.Endpoints.Endpoints {
			if extra.IsMRF {
				return nil, fmt.Errorf("%w: group %q uses MRF", ErrInvalidDefinition, group.ID)
			}
		}
		d.groups[group.ID] = group
		d.order = append(d.order, group.ID)
		if group.Stream != "" {
			d.streams[group.Stream] = append(d.streams[group.Stream], group.ID)
		}
	}
	for stream, members := range d.streams {
		first := d.groups[members[0]].Endpoints
		for _, id := range members[1:] {
			other := d.groups[id].Endpoints
			if first.BatchWait != other.BatchWait || first.BatchMaxSize != other.BatchMaxSize || first.BatchMaxContentSize != other.BatchMaxContentSize || first.Main.UseCompression != other.Main.UseCompression || first.Main.CompressionKind != other.Main.CompressionKind || first.Main.CompressionLevel != other.Main.CompressionLevel {
				return nil, fmt.Errorf("%w: incompatible batch settings in stream %q", ErrInvalidDefinition, stream)
			}
		}
	}
	return d, nil
}

// Groups returns the validated groups in declaration order.
func (d *Definitions) Groups() []Group {
	result := make([]Group, 0, len(d.order))
	for _, id := range d.order {
		result = append(result, d.groups[id])
	}
	return result
}

// Resolve checks a selection and removes repeated group IDs without admitting
// any implicit recipients. A declared shared stream must be selected in full.
func (d *Definitions) Resolve(selection Selection) ([]string, error) {
	if selection.Excluded {
		if len(selection.Groups) != 0 {
			return nil, ErrInvalidSelection
		}
		return nil, nil
	}
	if len(selection.Groups) == 0 {
		return nil, ErrInvalidSelection
	}
	selected := make(map[string]struct{}, len(selection.Groups))
	result := make([]string, 0, len(selection.Groups))
	for _, id := range selection.Groups {
		if _, exists := d.groups[id]; !exists {
			return nil, fmt.Errorf("%w: unknown group %q", ErrInvalidSelection, id)
		}
		if _, duplicate := selected[id]; !duplicate {
			selected[id] = struct{}{}
			result = append(result, id)
		}
	}
	for stream, members := range d.streams {
		count := 0
		for _, id := range members {
			if _, ok := selected[id]; ok {
				count++
			}
		}
		if count != 0 && count != len(members) {
			return nil, fmt.Errorf("%w: partial selection of shared stream %q", ErrInvalidSelection, stream)
		}
	}
	return result, nil
}

// Group returns a validated group definition.
func (d *Definitions) Group(id string) (Group, bool) {
	group, ok := d.groups[id]
	return group, ok
}

// StreamGroups returns the declared recipients of a shared stream.
func (d *Definitions) StreamGroups(stream string) []string {
	return append([]string(nil), d.streams[stream]...)
}
