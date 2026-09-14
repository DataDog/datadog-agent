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
// discovery-driven integration would use, since that's an indication that the
// user is already covering it manually.
var genericIntegrationCheckNames = map[string]struct{}{
	"openmetrics": {},
	"prometheus":  {},
}

// IsGenericIntegrationCheckName reports whether name is a generic integration
// check name.
func IsGenericIntegrationCheckName(name string) bool {
	_, ok := genericIntegrationCheckNames[name]
	return ok
}

// NamespaceRoot returns the portion of namespace before the first '.', or the
// whole string if there is none — e.g. "krakend.api" roots to "krakend".
func NamespaceRoot(namespace string) string {
	root, _, _ := strings.Cut(namespace, ".")
	return root
}

// ExpectedNamespaceRoot returns the metric-namespace root a discovery-driven
// integration's own metrics are expected to be published under: the root of
// its declared `discovery.metrics_prefix` (see integration.DiscoveryConfig)
// when set, or its own check name otherwise — true for the vast majority of
// integrations, with a small set of exceptions (e.g. zookeeper's namespace
// is "zk", not "zookeeper") that need metrics_prefix to be detected correctly.
//
// Only the root is used even when metrics_prefix is itself multi-segment
// (e.g. krakend's "krakend.api"): a generic scraper's own `namespace:` or
// metric rename could independently collide at a shorter prefix (e.g.
// `namespace: krakend` with the `api` part coming from the rename targets),
// so comparing only the root stays conservative.
func ExpectedNamespaceRoot(cfg integration.Config) string {
	if cfg.Discovery != nil && cfg.Discovery.MetricsPrefix != "" {
		return NamespaceRoot(cfg.Discovery.MetricsPrefix)
	}
	return cfg.Name
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
// unprefixed — so a rename target that's already a fully-qualified dotted name
// (e.g.  `envoy_cluster_http2_streams_active: envoy.cluster.http2.streams_active`)
// collides with the native integration's own metric, and there's no
// `namespace:` value to catch it.
//
// If an instance does not set an explicit namespace nor a qualifying rename,
// we ignore it since we don't have any way to to tell if it would produce
// conflicting metrics.
func GenericIntegrationNamespaceRoots(cfg integration.Config) []string {
	var roots []string
	for _, inst := range cfg.Instances {
		var common integration.CommonInstanceConfig
		if err := yaml.Unmarshal(inst, &common); err != nil {
			log.Debugf("Error while checking namespace root for %s, skipping instance: %v", cfg.Name, err)
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
// to a different name.  Each list entry is either
//   - a plain string: pass-through, not a rename, skipped; these involve wildcards
//     and we can't tell from the raw instance config alone whether that would
//     actually produce a metric that would collide with the native
//     integration's own metrics. However, OpenMetrics/Prometheus metrics names
//     traditionally do not contain dots (agent integrations only support them
//     with an undocumented option), so a raw metric name should not collide
//     with native integration's metrics, which do contain at least one dot
//     after the namespace root.
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
// generic scraper (openmetrics/prometheus) config that's already covering
// this service or host. Dropped when:
//  1. another check template (Instances > 0) for the same integration Name has
//     matched this same service (present in configs), or
//  2. any generic-scraper (openmetrics/prometheus) template has matched this
//     same service at all — regardless of its own Name or namespace. Once a
//     user has manually configured a generic scraper against a service, we
//     assume they already know how to collect its metrics, rather than try
//     to compare namespaces: comparing raw instance config can't reliably
//     rule out a collision (metrics could be passthrough/wildcard), so a
//     scoped comparison risks a false negative (failing to suppress a
//     duplicate that really would collide), or
//  3. a scheduled non-template (static) config exists for the same Name, or a
//     scheduled non-template generic-scraper config anywhere on the host
//     configures a namespace whose root matches this integration's expected
//     namespace root (see ExpectedNamespaceRoot) (both tracked, by check name
//     and by namespace root respectively, in the same staticIdx — see
//     configmgr.go). Unlike case 2, this host-wide static path stays
//     namespace-scoped: it isn't attached to any particular service, so a
//     blanket version of it would suppress discovery everywhere on the host
//     whenever a generic scraper happens to be configured anywhere.
//
// Logs-only sibling templates (no Instances) are ignored — discovery covers
// metric-check configuration and shouldn't be suppressed by an integration's
// log forwarding setup.
func filterTemplatesDiscovery(staticIdx *StaticConfigIndex, configs map[string]integration.Config) {
	if len(configs) == 0 {
		return
	}
	nonDiscoveryNames := map[string]struct{}{}
	hasGenericSibling := false
	for _, cfg := range configs {
		if cfg.IsDiscovery() || len(cfg.Instances) == 0 {
			continue
		}
		nonDiscoveryNames[cfg.Name] = struct{}{}
		if IsGenericIntegrationCheckName(cfg.Name) {
			hasGenericSibling = true
		}
	}
	for digest, cfg := range configs {
		if !cfg.IsDiscovery() {
			continue
		}
		_, hasSibling := nonDiscoveryNames[cfg.Name]
		expectedRoot := ExpectedNamespaceRoot(cfg)
		if hasGenericSibling || hasSibling || staticIdx.Has(cfg.Name) || (expectedRoot != cfg.Name && staticIdx.Has(expectedRoot)) {
			log.Debugf("Ignoring discovery template %s from %s: another config source already covers this integration",
				cfg.Name, cfg.Source)
			delete(configs, digest)
		}
	}
}
