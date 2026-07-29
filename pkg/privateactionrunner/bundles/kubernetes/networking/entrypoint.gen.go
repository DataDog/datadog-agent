// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_networking


import "github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"

type KubernetesNetworking struct {
	actions map[string]types.Action
}

func NewKubernetesNetworking() *KubernetesNetworking {
	return &KubernetesNetworking{
		actions: map[string]types.Action{
			// Manual actions
			// Auto-generated actions
			"createIngress":                 NewCreateIngressHandler(),
			"updateIngress":                 NewUpdateIngressHandler(),
			"deleteIngress":                 NewDeleteIngressHandler(),
			"deleteMultipleIngresses":       NewDeleteMultipleIngressesHandler(),
			"getIngress":                    NewGetIngressHandler(),
			"listIngress":                   NewListIngressHandler(),
			"patchIngress":                  NewPatchIngressHandler(),
			"createIngressClass":            NewCreateIngressClassHandler(),
			"updateIngressClass":            NewUpdateIngressClassHandler(),
			"deleteIngressClass":            NewDeleteIngressClassHandler(),
			"deleteMultipleIngressClasses":  NewDeleteMultipleIngressClassesHandler(),
			"getIngressClass":               NewGetIngressClassHandler(),
			"listIngressClass":              NewListIngressClassHandler(),
			"patchIngressClass":             NewPatchIngressClassHandler(),
			"createIPAddress":               NewCreateIPAddressHandler(),
			"updateIPAddress":               NewUpdateIPAddressHandler(),
			"deleteIPAddress":               NewDeleteIPAddressHandler(),
			"deleteMultipleIPAddresses":     NewDeleteMultipleIPAddressesHandler(),
			"getIPAddress":                  NewGetIPAddressHandler(),
			"listIPAddress":                 NewListIPAddressHandler(),
			"patchIPAddress":                NewPatchIPAddressHandler(),
			"createNetworkPolicy":           NewCreateNetworkPolicyHandler(),
			"updateNetworkPolicy":           NewUpdateNetworkPolicyHandler(),
			"deleteNetworkPolicy":           NewDeleteNetworkPolicyHandler(),
			"deleteMultipleNetworkPolicies": NewDeleteMultipleNetworkPoliciesHandler(),
			"getNetworkPolicy":              NewGetNetworkPolicyHandler(),
			"listNetworkPolicy":             NewListNetworkPolicyHandler(),
			"patchNetworkPolicy":            NewPatchNetworkPolicyHandler(),
			"createServiceCIDR":             NewCreateServiceCIDRHandler(),
			"updateServiceCIDR":             NewUpdateServiceCIDRHandler(),
			"deleteServiceCIDR":             NewDeleteServiceCIDRHandler(),
			"deleteMultipleServiceCIDRS":    NewDeleteMultipleServiceCIDRSHandler(),
			"getServiceCIDR":                NewGetServiceCIDRHandler(),
			"listServiceCIDR":               NewListServiceCIDRHandler(),
			"patchServiceCIDR":              NewPatchServiceCIDRHandler(),
		},
	}
}

func (h *KubernetesNetworking) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
