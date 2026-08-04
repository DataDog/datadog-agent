// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"slices"
	"strings"

	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FilterableService is an interface for a subset of services that can use advanced filtering
type FilterableService interface {
	// GetFilterableEntity returns the workloadmeta entity used for filtering, or nil if not available
	GetFilterableEntity() workloadfilter.Filterable
}

// filterTemplatesMatched removes any config that does not match the service's filterable entity
func filterTemplatesMatched(svc FilterableService, configs map[string]integration.Config) {
	filterableEntity := svc.GetFilterableEntity()
	if filterableEntity != nil {
		for digest, config := range configs {
			if !config.IsMatched(filterableEntity) {
				delete(configs, digest)
			}
		}
	}
}

// genericIntegrationCheckNames are check names whose entire configuration —
// including the metric namespace — is supplied directly by the user, rather
// than being intrinsic to a dedicated integration. These are commonly used as
// a fallback to collect metrics from services that don't (yet) have a
// dedicated Datadog integration. A configuration-discovery template is
// suppressed when one of these already claims the same metric namespace the
// discovery-driven integration would use, since that's a strong, specific
// signal the user is already covering it manually.
var genericIntegrationCheckNames = map[string]struct{}{
	"openmetrics": {},
	"prometheus":  {},
}

// IsGenericIntegrationCheckName reports whether name is a "generic"
// integration check name (openmetrics, prometheus) — see
// genericIntegrationCheckNames. Exported so the config manager can use the
// same check to decide which scheduled static configs to also track by
// namespace root in StaticConfigIndex.
func IsGenericIntegrationCheckName(name string) bool {
	_, ok := genericIntegrationCheckNames[name]
	return ok
}

// NamespaceRoot returns the portion of namespace before the first '.', or the
// whole string if there is none — e.g. "krakend.api" roots to "krakend". A
// discovery-driven integration's check name is assumed to equal its own
// metric namespace's root (true for the vast majority of integrations; a
// small, currently-accepted set of exceptions diverge — e.g. zk's own
// namespace is "zookeeper", not "zk"), so comparing a generic-scraper
// namespace's root against a discovery template's check name directly is
// enough to detect a conflict without a hand-maintained map.
func NamespaceRoot(namespace string) string {
	if i := strings.IndexByte(namespace, '.'); i >= 0 {
		return namespace[:i]
	}
	return namespace
}

// GenericIntegrationNamespaceRoots returns, for each instance in cfg, the
// metric-namespace root (see NamespaceRoot) it would submit metrics under:
//   - if the instance sets an explicit `namespace:`, that field's root, or
//   - otherwise, the root of each explicit metric rename target in the
//     instance's `metrics`/`extra_metrics` field (see
//     instanceMetricRenameTargets).
//
// The metrics-rename fallback only matters when namespace is unset: a
// generic openmetrics/prometheus check submits `namespace.metric_name`, but
// when namespace is empty the metric name is submitted completely
// unprefixed (verified in datadog_checks_base's AgentCheck._format_namespace)
// — so a rename target that's already a fully-qualified dotted name (e.g.
// `envoy_cluster_http2_streams_active: envoy.cluster.http2.streams_active`)
// collides with the native integration's own metric, and there's no
// `namespace:` value to catch it. When namespace *is* set, it's prepended on
// top of the rename target regardless, so the rename can't itself collide —
// hence checking metrics only in the no-namespace case.
//
// Instances with neither an explicit namespace nor a qualifying rename
// contribute nothing: with no signal to compare, assuming a match would risk
// suppressing discovery unnecessarily. Exported so the config manager can use
// the same logic to populate StaticConfigIndex with namespace roots from
// scheduled static (non-template) generic-scraper configs.
func GenericIntegrationNamespaceRoots(cfg integration.Config) []string {
	var roots []string
	for _, inst := range cfg.Instances {
		var common integration.CommonInstanceConfig
		if err := yaml.Unmarshal(inst, &common); err != nil {
			continue
		}
		if common.Namespace != "" {
			roots = append(roots, NamespaceRoot(common.Namespace))
			continue
		}
		for _, target := range instanceMetricRenameTargets(inst) {
			roots = append(roots, NamespaceRoot(target))
		}
	}
	return roots
}

// instanceMetricRenameTargets returns the explicit rename target of each
// entry in inst's `metrics`/`extra_metrics` field that renames a raw metric
// to a different name, mirroring the shapes accepted by
// MetricTransformer.normalize_metric_config (openmetrics v2) and the legacy
// metrics_mapper loops (openmetrics v1, prometheus) in datadog_checks_base:
// each list entry is either
//   - a plain string: pass-through, not a rename, skipped;
//   - a single-key map to a string: the string is the rename target; or
//   - a single-key map to a nested map with a `name` key: that key's value is
//     the rename target (no `name` key means the raw metric name is kept,
//     i.e. still not a rename, skipped).
func instanceMetricRenameTargets(inst integration.Data) []string {
	var raw struct {
		Metrics      []interface{} `yaml:"metrics"`
		ExtraMetrics []interface{} `yaml:"extra_metrics"`
	}
	if err := yaml.Unmarshal(inst, &raw); err != nil {
		return nil
	}
	var targets []string
	for _, entry := range slices.Concat(raw.Metrics, raw.ExtraMetrics) {
		m, ok := entry.(map[interface{}]interface{})
		if !ok {
			continue // plain string (or any other scalar): pass-through, no rename
		}
		for _, value := range m {
			switch v := value.(type) {
			case string:
				targets = append(targets, v)
			case map[interface{}]interface{}:
				if name, ok := v["name"].(string); ok {
					targets = append(targets, name)
				}
			}
		}
	}
	return targets
}

// filterTemplatesDiscovery drops configuration-discovery templates that are
// redundant with another config source for the same integration, or with a
// generic scraper (openmetrics/prometheus) config that's already claiming the
// same metric namespace. Dropped when:
//  1. another check template (Instances > 0) for the same integration Name has
//     matched this same service (present in configs), or
//  2. a sibling generic-scraper (openmetrics/prometheus) template matched to
//     this same service configures a namespace whose root matches this
//     integration's check name, or
//  3. a scheduled non-template (static) config exists for the same Name, or a
//     scheduled non-template generic-scraper config anywhere on the host
//     configures a namespace whose root matches this integration's check name
//     (both tracked, by check name and by namespace root respectively, in the
//     same staticIdx — see configmgr.go).
//
// Logs-only sibling templates (no Instances) are ignored — discovery covers
// metric-check configuration and shouldn't be suppressed by an integration's
// log forwarding setup.
func filterTemplatesDiscovery(staticIdx *StaticConfigIndex, configs map[string]integration.Config) {
	if len(configs) == 0 {
		return
	}
	nonDiscoveryNames := map[string]struct{}{}
	siblingGenericNamespaceRoots := map[string]struct{}{}
	for _, cfg := range configs {
		if cfg.IsDiscovery() || len(cfg.Instances) == 0 {
			continue
		}
		nonDiscoveryNames[cfg.Name] = struct{}{}
		if IsGenericIntegrationCheckName(cfg.Name) {
			for _, root := range GenericIntegrationNamespaceRoots(cfg) {
				siblingGenericNamespaceRoots[root] = struct{}{}
			}
		}
	}
	for digest, cfg := range configs {
		if !cfg.IsDiscovery() {
			continue
		}
		_, hasSibling := nonDiscoveryNames[cfg.Name]
		_, hasNamespaceConflict := siblingGenericNamespaceRoots[cfg.Name]
		if hasSibling || hasNamespaceConflict || staticIdx.Has(cfg.Name) {
			log.Debugf("Ignoring discovery template %s from %s: another config source already covers this integration",
				cfg.Name, cfg.Source)
			delete(configs, digest)
		}
	}
}
