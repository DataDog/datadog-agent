// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"strings"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
)

// SubscriptionSpec describes a gNMI path subscription with optional keyed segments.
type SubscriptionSpec struct {
	Path string
	Keys map[string]string // segment name -> key name
}

// String returns a human-readable subscription path.
func (s SubscriptionSpec) String() string {
	if len(s.Keys) == 0 {
		return s.Path
	}
	return s.Path + "{" + formatKeys(s.Keys) + "}"
}

// SubscriptionPaths returns the deduplicated gNMI paths the client subscribes to.
func SubscriptionPaths(cfg Config) []SubscriptionSpec {
	return buildSubscriptionSpecs(cfg)
}

// MetadataSubscriptionPaths returns metadata paths subscribed for device and interface metadata.
func MetadataSubscriptionPaths(metadata config.MetadataConfig) []SubscriptionSpec {
	specs := make([]SubscriptionSpec, 0, 16)
	for _, path := range metadata.Resolved().SubscriptionPaths() {
		specs = append(specs, SubscriptionSpec{
			Path: path.Path,
			Keys: path.Tags,
		})
	}
	return specs
}

// TopologySubscriptionPaths returns topology paths subscribed when topology collection is enabled.
func TopologySubscriptionPaths(topology config.TopologyConfig) []SubscriptionSpec {
	specs := make([]SubscriptionSpec, 0, 8)
	for _, path := range topology.SubscriptionPaths() {
		specs = append(specs, SubscriptionSpec{
			Path: path.Path,
			Keys: path.Tags,
		})
	}
	return specs
}

func buildSubscriptionSpecs(cfg Config) []SubscriptionSpec {
	metadataPaths := cfg.Profile.Metadata.Resolved().SubscriptionPaths()
	topologyPaths := cfg.Profile.Topology.SubscriptionPaths()
	specs := make([]SubscriptionSpec, 0, len(cfg.Profile.Metrics)+len(metadataPaths)+len(topologyPaths))
	seen := make(map[string]struct{})

	addSpec := func(spec SubscriptionSpec) {
		key := spec.Path + formatKeys(spec.Keys)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		specs = append(specs, spec)
	}

	for _, metric := range cfg.Profile.Metrics {
		addSpec(SubscriptionSpec{
			Path: normalizeSubscriptionPath(metric.Path),
			Keys: metric.SubscriptionKeys(),
		})
	}

	for _, path := range metadataPaths {
		addSpec(SubscriptionSpec{
			Path: path.Path,
			Keys: path.Tags,
		})
	}

	if cfg.CollectTopology {
		for _, path := range topologyPaths {
			addSpec(SubscriptionSpec{
				Path: path.Path,
				Keys: path.Tags,
			})
		}
	}

	return specs
}

func normalizeSubscriptionPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return trimmed
	}
	if trimmed[0] != '/' {
		return "/" + trimmed
	}
	return trimmed
}

func subscribePathFromSpec(spec SubscriptionSpec) (*gnmipb.Path, error) {
	segments, err := splitProfilePath(spec.Path)
	if err != nil {
		return nil, err
	}

	elems := make([]*gnmipb.PathElem, 0, len(segments))
	for _, segment := range segments {
		elem := &gnmipb.PathElem{Name: segment}
		if keyName, ok := spec.Keys[segment]; ok && keyName != "" {
			elem.Key = map[string]string{keyName: "*"}
		}
		elems = append(elems, elem)
	}

	return &gnmipb.Path{Elem: elems}, nil
}
