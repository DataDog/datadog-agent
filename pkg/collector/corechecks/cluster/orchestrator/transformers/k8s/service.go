// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	corev1 "k8s.io/api/core/v1"
)

// ExtractService returns the protobuf model corresponding to a Kubernetes
// Service resource.
func ExtractService(s *corev1.Service) *model.Service {
	message := &model.Service{
		Metadata: extractMetadata(&s.ObjectMeta),
		Spec: &model.ServiceSpec{
			ExternalIPs:              s.Spec.ExternalIPs,
			ExternalTrafficPolicy:    string(s.Spec.ExternalTrafficPolicy),
			PublishNotReadyAddresses: s.Spec.PublishNotReadyAddresses,
			SessionAffinity:          string(s.Spec.SessionAffinity),
			Type:                     string(s.Spec.Type),
		},
		Status: &model.ServiceStatus{},
	}

	if s.Spec.IPFamilies != nil {
		strFamilies := make([]string, len(s.Spec.IPFamilies))
		for i, fam := range s.Spec.IPFamilies {
			strFamilies[i] = string(fam)
		}
		message.Spec.IpFamily = strings.Join(strFamilies, ", ")
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

	message.Tags = append(message.Tags, transformers.RetrieveUnifiedServiceTags(s.ObjectMeta.Labels)...)

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
