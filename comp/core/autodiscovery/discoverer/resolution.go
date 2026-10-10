// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package discoverer

import (
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/configresolver"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ConfigDiscoveryTag is added to every instance of a check scheduled via
// configuration discovery. Configuration discovery has no way of knowing
// about a manually-configured, differently-named check (e.g. a generic
// `openmetrics` check) that a user has pointed at the same service from
// elsewhere, so the two can end up scraping the same target and duplicating
// (or, for additive metric types, doubling) submitted metrics. This tag lets
// users spot and exclude the autodiscovered side of such a duplication.
//
// Uses a plain `dd_`-prefixed key (not `dd.internal.*`, which is reserved for
// tags consumed and stripped internally before reaching the backend, e.g.
// dd.internal.resource in pkg/metrics/series.go) so it survives to the
// backend and stays queryable, following the precedent of other
// agent-added, customer-visible marker tags such as dd_remote_config_id /
// dd_remote_config_rev (comp/core/tagger/tags/tags.go) and
// dd_enable_check_intake (pkg/collector/worker/worker.go).
const ConfigDiscoveryTag = "dd_config_discovery:true"

// ResolveConfig merges the probe result into its template and resolves it
// against the current service. It does not access worker or config-manager state.
func (*Worker) ResolveConfig(template, discovered integration.Config, service listeners.Service) (integration.Config, error) {
	merged := template
	merged.Discovery = nil // Do not probe the already-discovered result again.
	merged.InitConfig = discovered.InitConfig
	merged.Instances = discovered.Instances
	merged.MetricConfig = discovered.MetricConfig
	merged.LogsConfig = discovered.LogsConfig
	merged.IgnoreAutodiscoveryTags = discovered.IgnoreAutodiscoveryTags
	merged.CheckTagCardinality = discovered.CheckTagCardinality

	resolved, err := configresolver.Resolve(merged, service)
	if err != nil {
		return integration.Config{}, err
	}
	resolved.Source = rewriteSource(resolved.Source, service)
	for i := range resolved.Instances {
		if err := resolved.Instances[i].MergeAdditionalTags([]string{ConfigDiscoveryTag}); err != nil {
			log.Errorf("error adding configuration-discovery tag to config %s for service %s: %v", resolved.Name, service.GetServiceID(), err)
		}
	}
	return resolved, nil
}

// rewriteSource rewrites a resolved config's file-based Source to encode that
// it was applied via a configuration-discovery probe result, and whether the
// target service is a process or a container. Only the "file" provider is
// rewritten since that's where we expect discovery configs to come from.
//
// This rewritten source is included in the configuration metadata sent to the
// backend.
//
// Config.Provider is intentionally left unchanged — it is used by the secret
// resolver security mechanism and must not vary with the service type.
func rewriteSource(source string, svc listeners.Service) string {
	if !strings.HasPrefix(source, names.File+":") {
		return source
	}
	if strings.HasPrefix(svc.GetServiceID(), "process://") {
		return names.ADProcessDiscovery + source[len(names.File):]
	}
	return names.ADContainerDiscovery + source[len(names.File):]
}
