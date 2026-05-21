// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"context"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1Client "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clusterInfoConfigMapName    = "dd-cluster-info"
	clusterInfoConfigMapDataKey = "cluster-info"
	clusterInfoManagedByLabel   = "app.kubernetes.io/managed-by"
	clusterInfoManagedByValue   = "kubectl-datadog"
	clusterInfoAPIVersion       = "v1"
)

// ClusterInfo mirrors the YAML payload of the dd-cluster-info ConfigMap
// produced by `kubectl datadog autoscaling cluster install`. The Go schema
// lives in datadog-operator; we replicate the YAML tags here so the agent
// does not depend on that repository.
type ClusterInfo struct {
	APIVersion     string                                 `json:"apiVersion"`
	ClusterName    string                                 `json:"clusterName"`
	ClusterARN     string                                 `json:"clusterArn,omitempty"`
	Region         string                                 `json:"region,omitempty"`
	GeneratedAt    time.Time                              `json:"generatedAt"`
	NodeManagement map[string]map[string]nodeManagerEntry `json:"nodeManagement"`
	Autoscaling    autoscalingYAML                        `json:"autoscaling"`
}

type nodeManagerEntry struct {
	Nodes            []string `json:"nodes"`
	ManagedByDatadog bool     `json:"managedByDatadog,omitempty"`
}

type autoscalingYAML struct {
	ClusterAutoscaler clusterAutoscalerYAML `json:"clusterAutoscaler"`
	Karpenter         karpenterYAML         `json:"karpenter"`
	EKSAutoMode       eksAutoModeYAML       `json:"eksAutoMode"`
}

type clusterAutoscalerYAML struct {
	Present   bool   `json:"present"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
}

type karpenterYAML struct {
	Present          bool   `json:"present"`
	Namespace        string `json:"namespace,omitempty"`
	Name             string `json:"name,omitempty"`
	Version          string `json:"version,omitempty"`
	ManagedByDatadog bool   `json:"managedByDatadog,omitempty"`
	InstallerVersion string `json:"installerVersion,omitempty"`
}

type eksAutoModeYAML struct {
	Enabled bool `json:"enabled"`
}

// FetchClusterInfo locates the dd-cluster-info ConfigMap anywhere in the
// cluster and returns its parsed payload. The ConfigMap is identified by
// its fixed name and the kubectl-datadog managed-by label. Returns
// (nil, nil) when no matching ConfigMap exists, when its schema version
// is not recognised, or when the payload key is missing — the absence of
// cluster-info is not an error, the snapshot is informational.
func FetchClusterInfo(ctx context.Context, coreClient corev1Client.CoreV1Interface) (*ClusterInfo, error) {
	cms, err := coreClient.ConfigMaps(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: clusterInfoManagedByLabel + "=" + clusterInfoManagedByValue,
		FieldSelector: "metadata.name=" + clusterInfoConfigMapName,
	})
	if err != nil {
		return nil, fmt.Errorf("listing %s ConfigMaps: %w", clusterInfoConfigMapName, err)
	}
	if len(cms.Items) == 0 {
		return nil, nil
	}
	sort.SliceStable(cms.Items, func(i, j int) bool {
		return cms.Items[i].Namespace < cms.Items[j].Namespace
	})
	if len(cms.Items) > 1 {
		namespaces := make([]string, 0, len(cms.Items))
		for i := range cms.Items {
			namespaces = append(namespaces, cms.Items[i].Namespace)
		}
		log.Warnc(fmt.Sprintf("found multiple %s ConfigMaps in namespaces %v, using %q",
			clusterInfoConfigMapName, namespaces, namespaces[0]), orchestrator.ExtraLogContext...)
	}
	chosen := &cms.Items[0]
	payload, ok := chosen.Data[clusterInfoConfigMapDataKey]
	if !ok {
		log.Warnc(fmt.Sprintf("%s ConfigMap in namespace %q is missing the %q data key",
			clusterInfoConfigMapName, chosen.Namespace, clusterInfoConfigMapDataKey),
			orchestrator.ExtraLogContext...)
		return nil, nil
	}
	var info ClusterInfo
	if err := yaml.Unmarshal([]byte(payload), &info); err != nil {
		return nil, fmt.Errorf("parsing %s ConfigMap payload: %w", clusterInfoConfigMapName, err)
	}
	if info.APIVersion != clusterInfoAPIVersion {
		log.Warnc(fmt.Sprintf("%s ConfigMap apiVersion %q is not supported (expected %q), ignoring",
			clusterInfoConfigMapName, info.APIVersion, clusterInfoAPIVersion),
			orchestrator.ExtraLogContext...)
		return nil, nil
	}
	return &info, nil
}

// ApplyClusterInfo enriches a cluster model and its node summaries with
// data sourced from the dd-cluster-info ConfigMap.
func ApplyClusterInfo(cluster *model.Cluster, nodesInfo []*model.ClusterNodeInfo, info *ClusterInfo, agentClusterName string) {
	if info == nil {
		return
	}

	if info.ClusterName != "" && agentClusterName != "" && info.ClusterName != agentClusterName {
		log.Warnc(fmt.Sprintf("dd-cluster-info clusterName %q does not match the agent cluster name %q",
			info.ClusterName, agentClusterName), orchestrator.ExtraLogContext...)
	}
	if info.Region != "" {
		for _, n := range nodesInfo {
			if n.Region != "" && n.Region != info.Region {
				log.Warnc(fmt.Sprintf("dd-cluster-info region %q does not match node %q region %q",
					info.Region, n.Name, n.Region), orchestrator.ExtraLogContext...)
				break
			}
		}
	}

	cluster.Autoscaling = toAutoscalingInfo(info.Autoscaling)
	if !info.GeneratedAt.IsZero() {
		cluster.ClusterInfoGeneratedAtUnixNano = info.GeneratedAt.UnixNano()
	}
	if info.ClusterARN != "" {
		cluster.CloudResourceId = &model.Cluster_Arn{Arn: info.ClusterARN}
	}

	nodeIndex := buildNodeManagerIndex(info.NodeManagement)
	for _, n := range nodesInfo {
		if entry, ok := nodeIndex[n.Name]; ok {
			n.NodeManager = entry.manager
			n.NodeManagerName = entry.name
			n.NodeManagerManagedByDatadog = entry.managedByDatadog
		}
	}
}

type nodeManagerAssignment struct {
	manager          string
	name             string
	managedByDatadog bool
}

// buildNodeManagerIndex inverts the dd-cluster-info nodeManagement
// structure (manager → entity name → entry) into a lookup by node name.
// Keys are iterated in sorted order so assignments stay deterministic
// across runs — fillClusterResourceVersion hashes the cluster JSON,
// non-deterministic iteration would defeat its change-detection cache.
func buildNodeManagerIndex(nodeManagement map[string]map[string]nodeManagerEntry) map[string]nodeManagerAssignment {
	out := make(map[string]nodeManagerAssignment)
	managers := make([]string, 0, len(nodeManagement))
	for m := range nodeManagement {
		managers = append(managers, m)
	}
	sort.Strings(managers)
	for _, manager := range managers {
		entries := nodeManagement[manager]
		names := make([]string, 0, len(entries))
		for n := range entries {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			entry := entries[name]
			for _, node := range entry.Nodes {
				if existing, ok := out[node]; ok {
					log.Warnc(fmt.Sprintf("node %q appears under multiple managers in dd-cluster-info (%s/%s and %s/%s); keeping the latter",
						node, existing.manager, existing.name, manager, name), orchestrator.ExtraLogContext...)
				}
				out[node] = nodeManagerAssignment{
					manager:          manager,
					name:             name,
					managedByDatadog: entry.ManagedByDatadog,
				}
			}
		}
	}
	return out
}

func toAutoscalingInfo(a autoscalingYAML) *model.AutoscalingInfo {
	return &model.AutoscalingInfo{
		ClusterAutoscaler: &model.ClusterAutoscalerInfo{
			Present:   a.ClusterAutoscaler.Present,
			Namespace: a.ClusterAutoscaler.Namespace,
			Name:      a.ClusterAutoscaler.Name,
			Version:   a.ClusterAutoscaler.Version,
		},
		Karpenter: &model.KarpenterInfo{
			Present:          a.Karpenter.Present,
			Namespace:        a.Karpenter.Namespace,
			Name:             a.Karpenter.Name,
			Version:          a.Karpenter.Version,
			ManagedByDatadog: a.Karpenter.ManagedByDatadog,
			InstallerVersion: a.Karpenter.InstallerVersion,
		},
		EksAutoMode: &model.EKSAutoModeInfo{
			Enabled: a.EKSAutoMode.Enabled,
		},
	}
}
