// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	networkingv1 "k8s.io/api/networking/v1"
)

// ExtractNetworkPolicy returns the protobuf model corresponding to a Kubernetes
func ExtractNetworkPolicy(n *networkingv1.NetworkPolicy) *model.NetworkPolicy {
	networkPolicy := model.NetworkPolicy{
		Metadata: extractMetadata(&n.ObjectMeta),
		Spec:     extractNetworkPolicySpec(&n.Spec),
	}

	networkPolicy.Tags = append(networkPolicy.Tags, transformers.RetrieveUnifiedServiceTags(n.ObjectMeta.Labels)...)

	return &networkPolicy
}

func extractNetworkPolicySpec(spec *networkingv1.NetworkPolicySpec) *model.NetworkPolicySpec {
	if spec == nil {
		return nil
	}

	policySpec := &model.NetworkPolicySpec{}

	if len(spec.PolicyTypes) > 0 {
		policySpec.PolicyTypes = extractNetworkPolicyTypes(spec.PolicyTypes)
	}

	if spec.PodSelector.Size() > 0 {
		policySpec.Selectors = extractLabelSelector(&spec.PodSelector)
	}

	if len(spec.Ingress) > 0 {
		policySpec.Ingress = make([]*model.NetworkPolicyIngressRule, 0, len(spec.Ingress))
		for _, i := range spec.Ingress {
			policySpec.Ingress = append(policySpec.Ingress, extractNetworkPolicyIngressRule(&i))
		}
	}

	if len(spec.Egress) > 0 {
		policySpec.Egress = make([]*model.NetworkPolicyEgressRule, 0, len(spec.Egress))
		for _, e := range spec.Egress {
			policySpec.Egress = append(policySpec.Egress, extractNetworkPolicyEgressRule(&e))
		}
	}

	return policySpec
}

func extractNetworkPolicyIngressRule(rule *networkingv1.NetworkPolicyIngressRule) *model.NetworkPolicyIngressRule {
	if rule == nil {
		return nil
	}

	from := make([]*model.NetworkPolicyPeer, 0, len(rule.From))
	for _, f := range rule.From {
		from = append(from, extractNetworkPolicyPeer(&f))
	}

	ports := make([]*model.NetworkPolicyPort, 0, len(rule.Ports))
	for _, p := range rule.Ports {
		ports = append(ports, extractNetworkPolicyPort(&p))
	}

	return &model.NetworkPolicyIngressRule{
		From:  from,
		Ports: ports,
	}
}

func extractNetworkPolicyEgressRule(rule *networkingv1.NetworkPolicyEgressRule) *model.NetworkPolicyEgressRule {
	if rule == nil {
		return nil
	}

	to := make([]*model.NetworkPolicyPeer, 0, len(rule.To))
	for _, t := range rule.To {
		to = append(to, extractNetworkPolicyPeer(&t))
	}

	ports := make([]*model.NetworkPolicyPort, 0, len(rule.Ports))
	for _, p := range rule.Ports {
		ports = append(ports, extractNetworkPolicyPort(&p))
	}

	return &model.NetworkPolicyEgressRule{
		To:    to,
		Ports: ports,
	}
}

func extractNetworkPolicyPeer(peer *networkingv1.NetworkPolicyPeer) *model.NetworkPolicyPeer {
	if peer == nil {
		return nil
	}

	ipBlock := extractNetworkPolicyIPBlock(peer.IPBlock)
	namespaceSelector := extractLabelSelector(peer.NamespaceSelector)
	podSelector := extractLabelSelector(peer.PodSelector)

	return &model.NetworkPolicyPeer{
		IpBlock:           ipBlock,
		NamespaceSelector: namespaceSelector,
		PodSelector:       podSelector,
	}
}

func extractNetworkPolicyIPBlock(ipBlock *networkingv1.IPBlock) *model.NetworkPolicyIPBlock {
	if ipBlock == nil {
		return nil
	}

	return &model.NetworkPolicyIPBlock{
		Cidr:   ipBlock.CIDR,
		Except: ipBlock.Except,
	}
}

func extractNetworkPolicyPort(port *networkingv1.NetworkPolicyPort) *model.NetworkPolicyPort {
	if port == nil {
		return nil
	}

	p := &model.NetworkPolicyPort{}
	if port.Port != nil {
		p.Port = port.Port.IntVal
	}

	if port.Protocol != nil {
		p.Protocol = string(*port.Protocol)
	}

	if port.EndPort != nil {
		p.EndPort = *port.EndPort
	}

	return p
}

func extractNetworkPolicyTypes(policyTypes []networkingv1.PolicyType) []string {
	if len(policyTypes) == 0 {
		return nil
	}

	types := make([]string, 0, len(policyTypes))
	for _, t := range policyTypes {
		types = append(types, string(t))
	}

	return types
}
