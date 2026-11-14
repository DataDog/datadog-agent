// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetValidatingWebhookStatus fetches the ValidatingWebhookConfiguration status.
var GetValidatingWebhookStatus = func(context.Context, string, kubernetes.Interface) (map[string]interface{}, error) {
	return nil, errors.New("admission controller not started")
}

// GetMutatingWebhookStatus fetches the MutatingWebhookConfiguration status.
var GetMutatingWebhookStatus = func(context.Context, string, kubernetes.Interface) (map[string]interface{}, error) {
	return nil, errors.New("admission controller not started")
}

// GetValidatingWebhookStatusV1 fetches a ValidatingWebhookConfiguration via the v1 API.
func GetValidatingWebhookStatusV1(ctx context.Context, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	validatingWebhookStatus := make(map[string]interface{})
	validatingWebhook, err := apiCl.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, err
	}

	validatingWebhookStatus["Name"] = validatingWebhook.GetName()
	validatingWebhookStatus["CreatedAt"] = validatingWebhook.GetCreationTimestamp()
	validatingWebhooksConfig := make(map[string]map[string]interface{})
	validatingWebhookStatus["Webhooks"] = validatingWebhooksConfig
	for _, w := range validatingWebhook.Webhooks {
		validatingWebhooksConfig[w.Name] = make(map[string]interface{})
		svc := w.ClientConfig.Service
		if svc != nil {
			port := "Port: None (default 443)"
			path := "Path: None"
			if svc.Port != nil {
				port = fmt.Sprintf("Port: %d", *svc.Port)
			}
			if svc.Path != nil {
				path = "Path: " + *svc.Path
			}
			validatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			validatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			validatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		validatingWebhooksConfig[w.Name]["CA bundle digest"] = Digest(w.ClientConfig.CABundle)
	}

	return validatingWebhookStatus, nil
}

// GetValidatingWebhookStatusV1beta1 fetches a ValidatingWebhookConfiguration via the v1beta1 API.
func GetValidatingWebhookStatusV1beta1(ctx context.Context, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	validatingWebhookStatus := make(map[string]interface{})
	validatingWebhook, err := apiCl.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, err
	}

	validatingWebhookStatus["Name"] = validatingWebhook.GetName()
	validatingWebhookStatus["CreatedAt"] = validatingWebhook.GetCreationTimestamp()
	validatingWebhooksConfig := make(map[string]map[string]interface{})
	validatingWebhookStatus["Webhooks"] = validatingWebhooksConfig
	for _, w := range validatingWebhook.Webhooks {
		validatingWebhooksConfig[w.Name] = make(map[string]interface{})
		svc := w.ClientConfig.Service
		if svc != nil {
			port := "Port: None (default 443)"
			path := "Path: None"
			if svc.Port != nil {
				port = fmt.Sprintf("Port: %d", *svc.Port)
			}
			if svc.Path != nil {
				path = "Path: " + *svc.Path
			}
			validatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			validatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			validatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		validatingWebhooksConfig[w.Name]["CA bundle digest"] = Digest(w.ClientConfig.CABundle)
	}

	return validatingWebhookStatus, nil
}

// GetMutatingWebhookStatusV1 fetches a MutatingWebhookConfiguration via the v1 API.
func GetMutatingWebhookStatusV1(ctx context.Context, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	mutatingWebhookStatus := make(map[string]interface{})
	mutatingWebhook, err := apiCl.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mutatingWebhookStatus, err
	}

	mutatingWebhookStatus["Name"] = mutatingWebhook.GetName()
	mutatingWebhookStatus["CreatedAt"] = mutatingWebhook.GetCreationTimestamp()
	mutatingWebhooksConfig := make(map[string]map[string]interface{})
	mutatingWebhookStatus["Webhooks"] = mutatingWebhooksConfig
	for _, w := range mutatingWebhook.Webhooks {
		mutatingWebhooksConfig[w.Name] = make(map[string]interface{})
		svc := w.ClientConfig.Service
		if svc != nil {
			port := "Port: None (default 443)"
			path := "Path: None"
			if svc.Port != nil {
				port = fmt.Sprintf("Port: %d", *svc.Port)
			}
			if svc.Path != nil {
				path = "Path: " + *svc.Path
			}
			mutatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			mutatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			mutatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		mutatingWebhooksConfig[w.Name]["CA bundle digest"] = Digest(w.ClientConfig.CABundle)
	}
	return mutatingWebhookStatus, nil
}

// GetMutatingWebhookStatusV1beta1 fetches a MutatingWebhookConfiguration via the v1beta1 API.
func GetMutatingWebhookStatusV1beta1(ctx context.Context, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	mutatingWebhookStatus := make(map[string]interface{})
	mutatingWebhook, err := apiCl.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return mutatingWebhookStatus, err
	}

	mutatingWebhookStatus["Name"] = mutatingWebhook.GetName()
	mutatingWebhookStatus["CreatedAt"] = mutatingWebhook.GetCreationTimestamp()
	mutatingWebhooksConfig := make(map[string]map[string]interface{})
	mutatingWebhookStatus["Webhooks"] = mutatingWebhooksConfig
	for _, w := range mutatingWebhook.Webhooks {
		mutatingWebhooksConfig[w.Name] = make(map[string]interface{})
		svc := w.ClientConfig.Service
		if svc != nil {
			port := "Port: None (default 443)"
			path := "Path: None"
			if svc.Path != nil {
				path = "Path: " + *svc.Path
			}
			if svc.Port != nil {
				port = fmt.Sprintf("Port: %d", *svc.Port)
			}
			mutatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			mutatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			mutatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		mutatingWebhooksConfig[w.Name]["CA bundle digest"] = Digest(w.ClientConfig.CABundle)
	}
	return mutatingWebhookStatus, nil
}

// Digest returns a short hash of b, used to display a certificate/CA bundle
// fingerprint without printing the raw bytes.
func Digest(b []byte) string {
	h := fnv.New64()
	_, _ = h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}
