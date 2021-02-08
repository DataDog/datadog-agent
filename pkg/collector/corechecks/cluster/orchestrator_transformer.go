// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package cluster

import (
	"fmt"
	"strings"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func extractDeployment(d *v1.Deployment) *model.Deployment {
	deploy := model.Deployment{
		Metadata: orchestrator.ExtractMetadata(&d.ObjectMeta),
	}
	// spec
	deploy.ReplicasDesired = 1 // default
	if d.Spec.Replicas != nil {
		deploy.ReplicasDesired = *d.Spec.Replicas
	}
	deploy.Paused = d.Spec.Paused
	deploy.DeploymentStrategy = string(d.Spec.Strategy.Type)
	if deploy.DeploymentStrategy == "RollingUpdate" && d.Spec.Strategy.RollingUpdate != nil {
		if d.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			deploy.MaxUnavailable = d.Spec.Strategy.RollingUpdate.MaxUnavailable.StrVal
		}
		if d.Spec.Strategy.RollingUpdate.MaxSurge != nil {
			deploy.MaxSurge = d.Spec.Strategy.RollingUpdate.MaxSurge.StrVal
		}
	}
	if d.Spec.Selector != nil {
		deploy.Selectors = extractLabelSelector(d.Spec.Selector)
	}

	// status
	deploy.Replicas = d.Status.Replicas
	deploy.UpdatedReplicas = d.Status.UpdatedReplicas
	deploy.ReadyReplicas = d.Status.ReadyReplicas
	deploy.AvailableReplicas = d.Status.AvailableReplicas
	deploy.UnavailableReplicas = d.Status.UnavailableReplicas
	deploy.ConditionMessage = extractDeploymentConditionMessage(d.Status.Conditions)

	return &deploy
}

func extractReplicaSet(rs *v1.ReplicaSet) *model.ReplicaSet {
	replicaSet := model.ReplicaSet{
		Metadata: orchestrator.ExtractMetadata(&rs.ObjectMeta),
	}
	// spec
	replicaSet.ReplicasDesired = 1 // default
	if rs.Spec.Replicas != nil {
		replicaSet.ReplicasDesired = *rs.Spec.Replicas
	}
	if rs.Spec.Selector != nil {
		replicaSet.Selectors = extractLabelSelector(rs.Spec.Selector)
	}

	// status
	replicaSet.Replicas = rs.Status.Replicas
	replicaSet.FullyLabeledReplicas = rs.Status.FullyLabeledReplicas
	replicaSet.ReadyReplicas = rs.Status.ReadyReplicas
	replicaSet.AvailableReplicas = rs.Status.AvailableReplicas

	return &replicaSet
}

// extractServiceMessage returns the protobuf Service message corresponding to
// a Kubernetes service object.
func extractService(s *corev1.Service) *model.Service {
	message := &model.Service{
		Metadata: orchestrator.ExtractMetadata(&s.ObjectMeta),
		Spec: &model.ServiceSpec{
			ExternalIPs:              s.Spec.ExternalIPs,
			ExternalTrafficPolicy:    string(s.Spec.ExternalTrafficPolicy),
			PublishNotReadyAddresses: s.Spec.PublishNotReadyAddresses,
			SessionAffinity:          string(s.Spec.SessionAffinity),
			Type:                     string(s.Spec.Type),
		},
		Status: &model.ServiceStatus{},
	}

	if s.Spec.IPFamily != nil {
		message.Spec.IpFamily = string(*s.Spec.IPFamily)
	}
	if s.Spec.SessionAffinityConfig != nil && s.Spec.SessionAffinityConfig.ClientIP != nil {
		message.Spec.SessionAffinityConfig = &model.ServiceSessionAffinityConfig{
			ClientIPTimeoutSeconds: *s.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds,
		}
	}
	if s.Spec.Type == corev1.ServiceTypeExternalName {
		message.Spec.ExternalName = s.Spec.ExternalName
	} else {
		message.Spec.ClusterIP = s.Spec.ClusterIP
	}
	if s.Spec.Type == corev1.ServiceTypeLoadBalancer {
		message.Spec.LoadBalancerIP = s.Spec.LoadBalancerIP
		message.Spec.LoadBalancerSourceRanges = s.Spec.LoadBalancerSourceRanges

		if s.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
			message.Spec.HealthCheckNodePort = s.Spec.HealthCheckNodePort
		}

		for _, ingress := range s.Status.LoadBalancer.Ingress {
			if ingress.Hostname != "" {
				message.Status.LoadBalancerIngress = append(message.Status.LoadBalancerIngress, ingress.Hostname)
			} else if ingress.IP != "" {
				message.Status.LoadBalancerIngress = append(message.Status.LoadBalancerIngress, ingress.IP)
			}
		}
	}

	if s.Spec.Selector != nil {
		message.Spec.Selectors = extractServiceSelector(s.Spec.Selector)
	}

	for _, port := range s.Spec.Ports {
		message.Spec.Ports = append(message.Spec.Ports, &model.ServicePort{
			Name:       port.Name,
			Protocol:   string(port.Protocol),
			Port:       port.Port,
			TargetPort: port.TargetPort.String(),
			NodePort:   port.NodePort,
		})
	}

	return message
}

func extractServiceSelector(ls map[string]string) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls))
	for k, v := range ls {
		labelSelectors = append(labelSelectors, &model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		})
	}
	return labelSelectors
}

func extractLabelSelector(ls *metav1.LabelSelector) []*model.LabelSelectorRequirement {
	labelSelectors := make([]*model.LabelSelectorRequirement, 0, len(ls.MatchLabels)+len(ls.MatchExpressions))
	for k, v := range ls.MatchLabels {
		s := model.LabelSelectorRequirement{
			Key:      k,
			Operator: "In",
			Values:   []string{v},
		}
		labelSelectors = append(labelSelectors, &s)
	}
	for _, s := range ls.MatchExpressions {
		sr := model.LabelSelectorRequirement{
			Key:      s.Key,
			Operator: string(s.Operator),
			Values:   s.Values,
		}
		labelSelectors = append(labelSelectors, &sr)
	}

	return labelSelectors
}

func extractDeploymentConditionMessage(conditions []v1.DeploymentCondition) string {
	messageMap := make(map[v1.DeploymentConditionType]string)

	// from https://github.com/kubernetes/kubernetes/blob/0b678bbb51a83e47df912f1205907418e354b281/staging/src/k8s.io/api/apps/v1/types.go#L417-L430
	// update if new ones appear
	chronologicalConditions := []v1.DeploymentConditionType{
		v1.DeploymentReplicaFailure,
		v1.DeploymentProgressing,
		v1.DeploymentAvailable,
	}

	// populate messageMap with messages for non-passing conditions
	for _, c := range conditions {
		if c.Status == corev1.ConditionFalse && c.Message != "" {
			messageMap[c.Type] = c.Message
		}
	}

	// return the message of the first one that failed
	for _, c := range chronologicalConditions {
		if m := messageMap[c]; m != "" {
			return m
		}
	}
	return ""
}

func extractNode(n *corev1.Node) *model.Node {
	msg := &model.Node{
		Metadata:      orchestrator.ExtractMetadata(&n.ObjectMeta),
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

	return msg
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
