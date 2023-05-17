// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
	netv1 "k8s.io/api/networking/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

// ExtractIngress returns the protobuf model corresponding to a Kubernetes
// Ingress resource.
func ExtractIngress(in *netv1.Ingress) *model.Ingress {
	ingress := model.Ingress{
		Metadata: extractMetadata(&in.ObjectMeta),
		Spec:     &model.IngressSpec{},
		Status:   &model.IngressStatus{},
	}

	if in.Spec.IngressClassName != nil {
		ingress.Spec.IngressClassName = *in.Spec.IngressClassName
	}

	if in.Spec.DefaultBackend != nil {
		ingress.Spec.DefaultBackend = extractIngressBackend(in.Spec.DefaultBackend)
	}

	if len(in.Spec.Rules) > 0 {
		ingress.Spec.Rules = extractIngressRules(in.Spec.Rules)
	}

	if len(in.Spec.TLS) > 0 {
		ingress.Spec.Tls = extractIngressTLS(in.Spec.TLS)
	}

	if len(in.Status.LoadBalancer.Ingress) > 0 {
		ingress.Status = extractIngressStatus(in.Status)
	}

	ingress.Tags = append(ingress.Tags, transformers.RetrieveUnifiedServiceTags(in.ObjectMeta.Labels)...)

	return &ingress
}

// extractIngressBackend converts a *netv1.IngressBackend object into *model.IngressBackend
func extractIngressBackend(ib *netv1.IngressBackend) *model.IngressBackend {
	ibModel := &model.IngressBackend{}

	if ib == nil {
		return ibModel
	}

	if ib.Resource != nil {
		ibModel.Resource = &model.TypedLocalObjectReference{
			Kind: ib.Resource.Kind,
			Name: ib.Resource.Name,
		}

		if ib.Resource.APIGroup != nil {
			ibModel.Resource.ApiGroup = *ib.Resource.APIGroup
		}
	}

	if ib.Service != nil {
		ibModel.Service = &model.IngressServiceBackend{
			ServiceName: ib.Service.Name,
			PortName:    ib.Service.Port.Name,
			PortNumber:  ib.Service.Port.Number,
		}
	}

	return ibModel
}

// extractIngressRules converts a []netv1.IngressRule object into []*model.IngressRule
func extractIngressRules(rules []netv1.IngressRule) []*model.IngressRule {
	rulesModel := make([]*model.IngressRule, len(rules))

	for i, rule := range rules {
		rulesModel[i] = &model.IngressRule{Host: rule.Host}

		if rule.IngressRuleValue.HTTP != nil {
			httpPaths := make([]*model.HTTPIngressPath, len(rule.IngressRuleValue.HTTP.Paths))
			for j, path := range rule.IngressRuleValue.HTTP.Paths {
				httpPaths[j] = &model.HTTPIngressPath{
					Backend: extractIngressBackend(&path.Backend),
					Path:    path.Path,
				}

				if path.PathType != nil {
					httpPaths[j].PathType = string(*path.PathType)
				}
			}

			rulesModel[i].HttpPaths = httpPaths
		}
	}

	return rulesModel
}

// extractIngressTLS converts a []netv1.IngressTLS object into []*model.IngressTLS
func extractIngressTLS(tls []netv1.IngressTLS) []*model.IngressTLS {
	tlsModel := make([]*model.IngressTLS, len(tls))

	for i, t := range tls {
		tlsModel[i] = &model.IngressTLS{
			Hosts:      t.Hosts,
			SecretName: t.SecretName,
		}
	}

	return tlsModel
}

// extractIngressStatus converts a netv1.IngressStatus object into *model.IngressStatus
func extractIngressStatus(status netv1.IngressStatus) *model.IngressStatus {
	statusModel := &model.IngressStatus{
		Ingress: make([]*model.LoadBalancerIngress, len(status.LoadBalancer.Ingress)),
	}

	for i, lbi := range status.LoadBalancer.Ingress {
		statusModel.Ingress[i] = &model.LoadBalancerIngress{
			Hostname: lbi.Hostname,
			Ip:       lbi.IP,
		}

		if len(lbi.Ports) > 0 {
			ports := make([]*model.PortStatus, len(lbi.Ports))
			for j, port := range lbi.Ports {
				ports[j] = &model.PortStatus{
					Port:     port.Port,
					Protocol: string(port.Protocol),
				}

				if port.Error != nil {
					ports[j].Error = *port.Error
				}
			}

			statusModel.Ingress[i].Ports = ports
		}
	}

	return statusModel
}
