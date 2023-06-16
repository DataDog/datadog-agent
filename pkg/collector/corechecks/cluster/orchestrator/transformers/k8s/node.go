// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

// ExtractNode returns the protobuf model corresponding to a Kubernetes Node
// resource.
func ExtractNode(n *corev1.Node) *model.Node {
	msg := &model.Node{
		Metadata:      extractMetadata(&n.ObjectMeta),
		PodCIDR:       n.Spec.PodCIDR,
		PodCIDRs:      n.Spec.PodCIDRs,
		ProviderID:    n.Spec.ProviderID,
		Unschedulable: n.Spec.Unschedulable,
		Status: &model.NodeStatus{
			Allocatable:             map[string]int64{},
			Capacity:                map[string]int64{},
			Architecture:            n.Status.NodeInfo.Architecture,
			ContainerRuntimeVersion: n.Status.NodeInfo.ContainerRuntimeVersion,
			OperatingSystem:         n.Status.NodeInfo.OperatingSystem,
			OsImage:                 n.Status.NodeInfo.OSImage,
			KernelVersion:           n.Status.NodeInfo.KernelVersion,
			KubeletVersion:          n.Status.NodeInfo.KubeletVersion,
			KubeProxyVersion:        n.Status.NodeInfo.KubeProxyVersion,
		},
	}

	if len(n.Spec.Taints) > 0 {
		msg.Taints = extractTaints(n.Spec.Taints)
	}

	extractCapacitiesAndAllocatables(n, msg)

	// extract status addresses
	if len(n.Status.Addresses) > 0 {
		msg.Status.NodeAddresses = map[string]string{}
		for _, address := range n.Status.Addresses {
			msg.Status.NodeAddresses[string(address.Type)] = address.Address
		}
	}

	// extract conditions
	for _, condition := range n.Status.Conditions {
		c := &model.NodeCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		}
		if !condition.LastTransitionTime.IsZero() {
			c.LastTransitionTime = condition.LastTransitionTime.Unix()
		}
		msg.Status.Conditions = append(msg.Status.Conditions, c)
	}

	// extract status message
	msg.Status.Status = computeNodeStatus(n)

	// extract role
	roles := findNodeRoles(n.Labels)
	if len(roles) > 0 {
		msg.Roles = roles
	}

	for _, image := range n.Status.Images {
		msg.Status.Images = append(msg.Status.Images, &model.ContainerImage{
			Names:     image.Names,
			SizeBytes: image.SizeBytes,
		})
	}

	addAdditionalNodeTags(msg)

	msg.Tags = append(msg.Tags, transformers.RetrieveUnifiedServiceTags(n.ObjectMeta.Labels)...)

	return msg
}

func addAdditionalNodeTags(nodeModel *model.Node) {
	// Add node status tags.
	additionalTags := convertNodeStatusToTags(nodeModel.Status.Status)

	// Add node role tag.
	for _, role := range nodeModel.Roles {
		additionalTags = append(additionalTags, fmt.Sprintf("%s:%s", kubernetes.KubeNodeRoleTagName, strings.ToLower(role)))
	}

	nodeModel.Tags = append(nodeModel.Tags, additionalTags...)
}

// computeNodeStatus is mostly copied from kubernetes to match what users see in kubectl
// in case of issues, check for changes upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1410
func computeNodeStatus(n *corev1.Node) string {
	conditionMap := make(map[corev1.NodeConditionType]*corev1.NodeCondition)
	NodeAllConditions := []corev1.NodeConditionType{corev1.NodeReady}
	for i := range n.Status.Conditions {
		cond := n.Status.Conditions[i]
		conditionMap[cond.Type] = &cond
	}
	var status []string
	for _, validCondition := range NodeAllConditions {
		if condition, ok := conditionMap[validCondition]; ok {
			if condition.Status == corev1.ConditionTrue {
				status = append(status, string(condition.Type))
			} else {
				status = append(status, "Not"+string(condition.Type))
			}
		}
	}
	if len(status) == 0 {
		status = append(status, "Unknown")
	}
	if n.Spec.Unschedulable {
		status = append(status, "SchedulingDisabled")
	}
	return strings.Join(status, ",")
}

func convertNodeStatusToTags(nodeStatus string) []string {
	var tags []string
	unschedulable := false
	for _, status := range strings.Split(nodeStatus, ",") {
		if status == "" {
			continue
		}
		if status == "SchedulingDisabled" {
			unschedulable = true
			tags = append(tags, "node_schedulable:false")
			continue
		}
		tags = append(tags, fmt.Sprintf("node_status:%s", strings.ToLower(status)))
	}
	if !unschedulable {
		tags = append(tags, "node_schedulable:true")
	}
	return tags
}

func extractCapacitiesAndAllocatables(n *corev1.Node, mn *model.Node) {
	// Milli Value ceil(q * 1000), which fits to be the lowest value. CPU -> Millicore and Memory -> byte
	supportedResourcesMilli := []corev1.ResourceName{corev1.ResourceCPU}
	supportedResources := []corev1.ResourceName{corev1.ResourcePods, corev1.ResourceMemory}
	setSupportedResources(n, mn, supportedResources, false)
	setSupportedResources(n, mn, supportedResourcesMilli, true)
}

func setSupportedResources(n *corev1.Node, mn *model.Node, supportedResources []corev1.ResourceName, isMilli bool) {
	for _, resource := range supportedResources {
		capacity, hasCapacity := n.Status.Capacity[resource]
		if hasCapacity && !capacity.IsZero() {
			if isMilli {
				mn.Status.Capacity[resource.String()] = capacity.MilliValue()
			} else {
				mn.Status.Capacity[resource.String()] = capacity.Value()
			}
		}
		allocatable, hasAllocatable := n.Status.Allocatable[resource]

		if hasAllocatable && !allocatable.IsZero() {
			if isMilli {
				mn.Status.Allocatable[resource.String()] = allocatable.MilliValue()
			} else {
				mn.Status.Allocatable[resource.String()] = allocatable.Value()
			}
		}
	}
}

func extractTaints(taints []corev1.Taint) []*model.Taint {
	modelTaints := make([]*model.Taint, 0, len(taints))

	for _, taint := range taints {
		modelTaint := &model.Taint{
			Key:    taint.Key,
			Value:  taint.Value,
			Effect: string(taint.Effect),
		}
		if !taint.TimeAdded.IsZero() {
			modelTaint.TimeAdded = taint.TimeAdded.Unix()
		}
		modelTaints = append(modelTaints, modelTaint)
	}
	return modelTaints
}

// findNodeRoles returns the roles of a given node.
// The roles are determined by looking for:
// * a node-role.kubernetes.io/<role>="" label
// * a kubernetes.io/role="<role>" label
// is mostly copied from kubernetes, for issues check upstream: https://github.com/kubernetes/kubernetes/blob/1e12d92a5179dbfeb455c79dbf9120c8536e5f9c/pkg/printers/internalversion/printers.go#L1487
func findNodeRoles(nodeLabels map[string]string) []string {
	labelNodeRolePrefix := "node-role.kubernetes.io/"
	nodeLabelRole := "kubernetes.io/role"

	roles := sets.NewString()
	for k, v := range nodeLabels {
		switch {
		case strings.HasPrefix(k, labelNodeRolePrefix):
			if role := strings.TrimPrefix(k, labelNodeRolePrefix); len(role) > 0 {
				roles.Insert(role)
			}

		case k == nodeLabelRole && v != "":
			roles.Insert(v)
		}
	}
	return roles.List()
}
