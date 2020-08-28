// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package orchestrator

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util/orchestrator"
	"k8s.io/apimachinery/pkg/version"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func extractCluster(nodeList []*corev1.Node, nsList []*corev1.Namespace, clusterName string, clusterID string, serverApiVersion *version.Info) *model.Cluster {
	allocatables, capacities, kubeletVersions := extractNodeInformation(nodeList)
	cluster := model.Cluster{
		Name:              clusterName,
		Uid:               clusterID,
		NamespaceCount:    int32(len(nsList)),
		NodeCount:         int32(len(nodeList)),
		KubeletVersions:   kubeletVersions,
		ApiServerVersions: serverApiVersion.String(),
		Allocatable:       allocatables,
		Capacity:          capacities,
	}
	return &cluster
}

// TODO: think about caching and invalidation strategies
// max 5000 nodes, usually below and they are usually not that volatile. We may want to use the resourceVersion for caching.
func extractNodeInformation(nodeList []*corev1.Node) (clusterCapacity map[string]int64, clusterAllocatable map[string]int64, kubeletVersions map[string]int32) {
	kubeletVersions = make(map[string]int32)
	clusterCapacity = make(map[string]int64)
	clusterAllocatable = make(map[string]int64)
	allocatablePods := int64(0)
	allocatableCpu := int64(0)
	allocatableMem := int64(0)
	capacityMem := int64(0)
	capacityPods := int64(0)
	capacityCpu := int64(0)
	for _, node := range nodeList {
		kubeletVersion := node.Status.NodeInfo.KubeletVersion
		// pods are given as normal values 1 pod, 2 pods. Hence, Value().
		allocatablePods += node.Status.Allocatable.Pods().Value()
		allocatableCpu += node.Status.Allocatable.Cpu().MilliValue()
		allocatableMem += node.Status.Allocatable.Memory().MilliValue()

		capacityCpu += node.Status.Capacity.Cpu().MilliValue()
		capacityMem += node.Status.Capacity.Memory().MilliValue()
		capacityPods += node.Status.Capacity.Pods().Value()
		kubeletVersions[kubeletVersion] += 1
	}
	clusterCapacity[string(corev1.ResourceCPU)] = capacityCpu
	clusterCapacity[string(corev1.ResourceMemory)] = capacityMem
	clusterCapacity[string(corev1.ResourcePods)] = capacityPods
	clusterAllocatable[string(corev1.ResourcePods)] = allocatablePods
	clusterAllocatable[string(corev1.ResourceMemory)] = allocatableMem
	clusterAllocatable[string(corev1.ResourceCPU)] = allocatableCpu
	return
}
