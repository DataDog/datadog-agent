// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"context"
	"fmt"
	"io"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	apiServerCommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LocalWorkloadMetricStoreInfo is the response type of the autoscaling workload check.
type LocalWorkloadMetricStoreInfo struct {
	LocalAutoscalingWorkloadEntities []LocalAutoscalingWorkloadEntity
}

// LocalAutoscalingWorkloadEntity has status data for local autoscaling workload.
type LocalAutoscalingWorkloadEntity map[string]interface{}

func defaultDisabledNamespaces() map[string]struct{} {
	disabledNamespaces := make(map[string]struct{})
	disabledNamespaces[apiServerCommon.GetResourcesNamespace()] = struct{}{}
	return disabledNamespaces
}

// GetAutoscalingWorkloadCheck retrieves the autoscaling workload check response, from the local store and recommendation store.
func GetAutoscalingWorkloadCheck(ctx context.Context) *LocalWorkloadMetricStoreInfo {

	resp := LocalWorkloadMetricStoreInfo{
		LocalAutoscalingWorkloadEntities: make([]LocalAutoscalingWorkloadEntity, 0),
	}

	localCheck := getLocalAutoscalingWorkloadCheck(ctx)
	if localCheck != nil {
		resp.LocalAutoscalingWorkloadEntities = localCheck
	}
	return &resp
}

func getLocalAutoscalingWorkloadCheck(ctx context.Context) []LocalAutoscalingWorkloadEntity {
	if ctx == nil || !pkgconfigsetup.Datadog().GetBool("autoscaling.failover.enabled") {
		return nil
	}
	result := make([]LocalAutoscalingWorkloadEntity, 0)
	lStore := GetWorkloadMetricStore(ctx)
	lStoreInfo := lStore.GetStoreInfo()
	if len(lStoreInfo.StatsResults) == 0 {
		log.Infof("No local autoscaling entities found")
		return result
	}
	for _, statsResult := range lStoreInfo.StatsResults {
		// Skip the disabled namespaces
		if _, ok := defaultDisabledNamespaces()[statsResult.Namespace]; ok {
			log.Debugf("Skipping local autoscaling entity in disabled namespace: %s", statsResult.Namespace)
			continue
		}
		result = append(result, LocalAutoscalingWorkloadEntity{
			"Namespace":            statsResult.Namespace,
			"PodOwner":             statsResult.PodOwner,
			"MetricName":           statsResult.MetricName,
			"Datapoints(PodLevel)": statsResult.Count,
		})
	}
	return result
}

// Dump writes the autoscaling workload check response to the given writer.
func (response *LocalWorkloadMetricStoreInfo) Dump(w io.Writer) {
	for _, workloadEntity := range response.LocalAutoscalingWorkloadEntities {
		namespace, ok := workloadEntity["Namespace"]
		if !ok || namespace == nil {
			continue
		}

		podOwner, ok := workloadEntity["PodOwner"]
		if !ok || podOwner == nil {
			continue
		}

		metricName, ok := workloadEntity["MetricName"]
		if !ok || metricName == nil {
			continue
		}

		datapoints, ok := workloadEntity["Datapoints(PodLevel)"]
		if !ok || datapoints == nil {
			continue
		}
		fmt.Fprintf(w, "Namespace: %s, PodOwner: %s, MetricName: %s, Datapoints: %v\n",
			namespace, podOwner, metricName, datapoints,
		)
	}
}
