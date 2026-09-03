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

// MetadataSubscriptionPaths returns OpenConfig paths subscribed for device and interface metadata.
func MetadataSubscriptionPaths() []SubscriptionSpec {
	return []SubscriptionSpec{
		{Path: "/system/state/hostname"},
		{Path: "/system/state/vendor-name"},
		{Path: "/system/state/serial-number"},
		{Path: "/system/state/platform"},
		{Path: "/system/state/software-version"},
		{Path: "/system/state/hardware-version"},
		{Path: "/interfaces/interface/state/name", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/description", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/admin-status", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/oper-status", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/mac-address", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/ifindex", Keys: map[string]string{"interface": "name"}},
		{Path: "/interfaces/interface/state/type", Keys: map[string]string{"interface": "name"}},
	}
}

// TopologySubscriptionPaths returns OpenConfig LLDP paths subscribed when topology collection is enabled.
func TopologySubscriptionPaths() []SubscriptionSpec {
	return []SubscriptionSpec{
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/chassis-id-type", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/port-id", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/port-id-type", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/system-name", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/system-description", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/port-description", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
		{Path: "/lldp/interfaces/interface/neighbors/neighbor/state/management-address", Keys: map[string]string{"interface": "name", "neighbor": "id"}},
	}
}

func buildSubscriptionSpecs(cfg Config) []SubscriptionSpec {
	specs := make([]SubscriptionSpec, 0, len(cfg.Profile.Metrics)+len(MetadataSubscriptionPaths()))
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
			Keys: metricKeyWildcards(metric),
		})
	}

	for _, spec := range MetadataSubscriptionPaths() {
		addSpec(spec)
	}

	if cfg.CollectTopology {
		for _, spec := range TopologySubscriptionPaths() {
			addSpec(spec)
		}
	}

	return specs
}

func metricKeyWildcards(metric config.MetricConfig) map[string]string {
	if len(metric.Tags) == 0 {
		return nil
	}
	keys := make(map[string]string, len(metric.Tags))
	for segment, keyName := range metric.Tags {
		if keyName == "" {
			continue
		}
		keys[segment] = keyName
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
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
