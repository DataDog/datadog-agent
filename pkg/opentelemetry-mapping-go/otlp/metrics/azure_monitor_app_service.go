// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"math"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

const (
	azureMonitorScope                    = "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/azuremonitorreceiver"
	azureMonitorAppServiceActiveInstance = "otel.datadog_exporter.metrics.azuremonitor.app_service.active_instance"
)

// Include the timestamp in the key: one scrape can contain several minutes, and
// collapsing them to export time would move activity into a different usage window.
// A struct avoids collisions between identities containing separators.
type azureMonitorAppServiceObservation struct {
	subscriptionID string
	resourceGroup  string
	name           string
	instance       string
	timestamp      pcommon.Timestamp
}

func consumeAzureMonitorAppServiceMetrics(ctx context.Context, md pmetric.Metrics, consumer Consumer, originProduct OriginProduct) {
	seen := make(map[azureMonitorAppServiceObservation]struct{})
	for _, rm := range md.ResourceMetrics().All() {
		for _, sm := range rm.ScopeMetrics().All() {
			if sm.Scope().Name() != azureMonitorScope {
				continue
			}
			for _, m := range sm.Metrics().All() {
				// Count counts samples, not CPU activity. Other aggregations and
				// Functions metrics are intentionally outside this proof's contract.
				if m.Name() != "azure_cputime_total" || m.Type() != pmetric.MetricTypeGauge {
					continue
				}
				for _, dp := range m.Gauge().DataPoints().All() {
					if dp.Timestamp() == 0 || dp.Flags().NoRecordedValue() {
						continue
					}
					var value float64
					switch dp.ValueType() {
					case pmetric.NumberDataPointValueTypeInt:
						value = float64(dp.IntValue())
					case pmetric.NumberDataPointValueTypeDouble:
						value = dp.DoubleValue()
					}
					if !(value > 0) || math.IsInf(value, 0) {
						continue
					}
					observation, ok := azureMonitorAppServiceIdentity(dp.Attributes())
					if !ok {
						continue
					}
					observation.timestamp = dp.Timestamp()
					if _, exists := seen[observation]; exists {
						continue
					}
					seen[observation] = struct{}{}
					// Do not inherit Collector host, resource tags, or origin ID.
					// TagSetConsumer cannot preserve cloud observation timestamps.
					consumer.ConsumeTimeSeries(ctx, &Dimensions{
						name: azureMonitorAppServiceActiveInstance,
						tags: []string{
							"subscription_id:" + observation.subscriptionID,
							"resource_group:" + observation.resourceGroup,
							"name:" + observation.name,
							"instance:" + observation.instance,
						},
						originProduct:       originProduct,
						originSubProduct:    OriginSubProductOTLP,
						originProductDetail: OriginProductDetailAzureMonitorReceiver,
					}, Gauge, uint64(dp.Timestamp()), 0, 1)
				}
			}
		}
	}
}

func azureMonitorAppServiceIdentity(attrs pcommon.Map) (azureMonitorAppServiceObservation, bool) {
	// The subscription resource attribute is optional in the receiver. Use the
	// datapoint ARM ID instead, and reject slots, plans, or conflicting labels.
	parts := strings.Split(azureMonitorIdentityAttribute(attrs, "azuremonitor.resource_id"), "/")
	if len(parts) != 9 || parts[0] != "" || parts[1] != "subscriptions" || parts[2] == "" || strings.TrimSpace(parts[2]) != parts[2] ||
		parts[3] != "resourcegroups" || parts[4] == "" || parts[5] != "providers" ||
		parts[6] != "microsoft.web" || parts[7] != "sites" || parts[8] == "" {
		return azureMonitorAppServiceObservation{}, false
	}
	identity := azureMonitorAppServiceObservation{
		subscriptionID: parts[2],
		resourceGroup:  parts[4],
		name:           parts[8],
		instance:       azureMonitorIdentityAttribute(attrs, "metadata_Instance"),
	}
	if identity.instance == "" || azureMonitorIdentityAttribute(attrs, "type") != "microsoft.web/sites" ||
		azureMonitorIdentityAttribute(attrs, "resource_group") != identity.resourceGroup ||
		azureMonitorIdentityAttribute(attrs, "name") != identity.name {
		return azureMonitorAppServiceObservation{}, false
	}
	return identity, true
}

func azureMonitorIdentityAttribute(attrs pcommon.Map, key string) string {
	v, ok := attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeStr || strings.TrimSpace(v.Str()) != v.Str() {
		return ""
	}
	return strings.ToLower(v.Str())
}
