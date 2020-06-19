// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package admission

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetStatus returns status info for the secret and webhook controllers.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !config.Datadog.GetBool("admission_controller.enabled") {
		status["Disabled"] = "The admission controller is not enabled on the Cluster Agent"
		return status
	}

	ns := common.GetResourcesNamespace()
	webhookName := config.Datadog.GetString("admission_controller.webhook_name")
	secretName := config.Datadog.GetString("admission_controller.certificate.secret_name")
	status["WebhookName"] = webhookName
	status["SecretName"] = fmt.Sprintf("%s/%s", ns, secretName)

	webhookStatus, err := getWebhookStatus(webhookName, apiCl)
	if err != nil {
		status["WebhookError"] = err.Error()
	} else {
		status["Webhooks"] = webhookStatus
	}

	secretStatus, err := getSecretStatus(ns, secretName, apiCl)
	if err != nil {
		status["SecretError"] = err.Error()
	} else {
		status["Secret"] = secretStatus
	}

	return status
}

func getWebhookStatus(name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	webhookStatus := make(map[string]interface{})
	webhook, err := apiCl.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(name, metav1.GetOptions{})
	if err != nil {
		return webhookStatus, err
	}

	webhookStatus["Name"] = webhook.GetName()
	webhookStatus["CreatedAt"] = webhook.GetCreationTimestamp()

	webhooksConfig := make(map[string]map[string]interface{})
	webhookStatus["Webhooks"] = webhooksConfig
	for _, w := range webhook.Webhooks {
		webhooksConfig[w.Name] = make(map[string]interface{})
		svc := w.ClientConfig.Service
		if svc != nil {
			webhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - Port: %d - Path: %s", svc.Namespace, svc.Name, *svc.Port, *svc.Path)
		}
		if w.ObjectSelector != nil {
			webhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			webhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		webhooksConfig[w.Name]["CA bundle digest"] = getDigest(w.ClientConfig.CABundle)
	}
	return webhookStatus, nil
}

func getSecretStatus(ns, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	secretStatus := make(map[string]interface{})
	secret, err := apiCl.CoreV1().Secrets(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return secretStatus, err
	}
	secretStatus["Name"] = secret.GetName()
	secretStatus["Namespace"] = secret.GetNamespace()
	secretStatus["CreatedAt"] = secret.GetCreationTimestamp()
	secretStatus["CABundleDigest"] = getDigest(secret.Data["cert.pem"])
	t, err := certificate.GetDurationBeforeExpiration(secret.Data)
	if err != nil {
		log.Errorf("Cannot get certificate validity duration: %v", err)
	}
	secretStatus["CertValidDuration"] = t.String()
	return secretStatus, nil
}

func getDigest(b []byte) string {
	h := fnv.New64()
	_, _ = h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}
