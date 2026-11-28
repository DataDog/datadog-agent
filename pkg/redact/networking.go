// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package redact

import (
	networkingv1 "k8s.io/api/networking/v1"
)

// ScrubIngress scrubs the metadata of a Ingress.
func ScrubIngress(r *networkingv1.Ingress, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubIngressClass scrubs the metadata of a IngressClass.
func ScrubIngressClass(r *networkingv1.IngressClass, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubIPAddress scrubs the metadata of a IPAddress.
func ScrubIPAddress(r *networkingv1.IPAddress, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubNetworkPolicy scrubs the metadata of a NetworkPolicy.
func ScrubNetworkPolicy(r *networkingv1.NetworkPolicy, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}

// ScrubServiceCIDR scrubs the metadata of a ServiceCIDR.
func ScrubServiceCIDR(r *networkingv1.ServiceCIDR, scrubber *DataScrubber) {
	ScrubObjectMeta(&r.ObjectMeta, scrubber)
}
