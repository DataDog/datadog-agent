// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"fmt"

	osq "github.com/openshift/api/quota/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// reportClusterQuotas reports metrics on OpenShift ClusterResourceQuota objects
func (k *KubeASCheck) reportClusterQuotas(quotas []osq.ClusterResourceQuota, sender aggregator.Sender) {
	for _, quota := range quotas {
		quotaTags := append(k.instance.Tags, fmt.Sprintf("clusterquota:%s", quota.Name))
		remaining := computeQuotaRemaining(quota.Status.Total.Used, quota.Status.Total.Hard)

		k.reportQuota(quota.Status.Total.Hard, "openshift.clusterquota", "limit", quotaTags, sender)
		k.reportQuota(quota.Status.Total.Used, "openshift.clusterquota", "used", quotaTags, sender)
		k.reportQuota(remaining, "openshift.clusterquota", "remaining", quotaTags, sender)

		for _, nsQuota := range quota.Status.Namespaces {
			nsTags := append(quotaTags, fmt.Sprintf("kube_namespace:%s", nsQuota.Namespace))
			k.reportQuota(nsQuota.Status.Hard, "openshift.appliedclusterquota", "limit", nsTags, sender)
			k.reportQuota(nsQuota.Status.Used, "openshift.appliedclusterquota", "used", nsTags, sender)
			k.reportQuota(remaining, "openshift.appliedclusterquota", "remaining", nsTags, sender)
		}

	}
}

func (k *KubeASCheck) reportQuota(quotas v1.ResourceList, metricPrefix, metricSuffix string, tags []string, sender aggregator.Sender) {
	for res, qty := range quotas {
		metricName := fmt.Sprintf("%s.%s.%s", metricPrefix, res, metricSuffix)
		sender.Gauge(metricName, quantityToFloat64(qty), "", tags)
	}
}

func quantityToFloat64(qty resource.Quantity) float64 {
	return float64(qty.MilliValue()) / 1000
}

func computeQuotaRemaining(used, limit v1.ResourceList) v1.ResourceList {
	// Map values are not addressable as pointers, need to create an
	// intermediate map of custom type to be able to subtract
	remaining := make(map[v1.ResourceName]*resource.Quantity)

	for res, qty := range limit {
		remaining[res] = qty.Copy()
	}
	for res, qty := range used {
		ptr := remaining[res]
		if ptr == nil {
			log.Debugf("Resource %s: has a usage but no limit, skipping remaining computation", res)
			continue
		}
		ptr.Sub(qty)
	}

	output := make(v1.ResourceList)
	for res, ptr := range remaining {
		if ptr != nil {
			output[res] = *ptr
		}
	}
	return output
}
