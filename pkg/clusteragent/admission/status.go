// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package admission

import (
	"context"
	"embed"
	"fmt"
	"hash/fnv"
	"io"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/certificate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetStatus returns status info for the secret and webhook controllers.
func GetStatus(apiCl kubernetes.Interface) map[string]interface{} {
	status := make(map[string]interface{})
	if !pkgconfigsetup.Datadog().GetBool("admission_controller.enabled") {
		status["Disabled"] = "The admission controller is not enabled on the Cluster Agent"
		return status
	}

	ns := common.GetResourcesNamespace()
	webhookName := pkgconfigsetup.Datadog().GetString("admission_controller.webhook_name")
	secretName := pkgconfigsetup.Datadog().GetString("admission_controller.certificate.secret_name")
	status["WebhookName"] = webhookName
	status["SecretName"] = fmt.Sprintf("%s/%s", ns, secretName)

	validatingWebhookStatus, mutatingWebhookStatus, err := getWebhookStatus(webhookName, apiCl)
	if err != nil {
		status["WebhookError"] = err.Error()
	} else {
		status["ValidatingWebhooks"] = validatingWebhookStatus
		status["MutatingWebhooks"] = mutatingWebhookStatus
	}

	secretStatus, err := getSecretStatus(ns, secretName, apiCl)
	if err != nil {
		status["SecretError"] = err.Error()
	} else {
		status["Secret"] = secretStatus
	}

	return status
}

var getWebhookStatus = func(string, kubernetes.Interface) (map[string]interface{}, map[string]interface{}, error) {
	return nil, nil, fmt.Errorf("admission controller not started")
}

func getWebhookStatusV1(name string, apiCl kubernetes.Interface) (map[string]interface{}, map[string]interface{}, error) {
	validatingWebhookStatus, mutatingWebhookStatus := make(map[string]interface{}), make(map[string]interface{})
	validatingWebhook, err := apiCl.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, mutatingWebhookStatus, err
	}
	mutatingWebhook, err := apiCl.AdmissionregistrationV1().MutatingWebhookConfigurations().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, mutatingWebhookStatus, err
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
				path = fmt.Sprintf("Path: %s", *svc.Path)
			}
			validatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			validatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			validatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		validatingWebhooksConfig[w.Name]["CA bundle digest"] = getDigest(w.ClientConfig.CABundle)
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
				path = fmt.Sprintf("Path: %s", *svc.Path)
			}
			mutatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			mutatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			mutatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		mutatingWebhooksConfig[w.Name]["CA bundle digest"] = getDigest(w.ClientConfig.CABundle)
	}
	return validatingWebhookStatus, mutatingWebhookStatus, nil
}

func getWebhookStatusV1beta1(name string, apiCl kubernetes.Interface) (map[string]interface{}, map[string]interface{}, error) {
	validatingWebhookStatus, mutatingWebhookStatus := make(map[string]interface{}), make(map[string]interface{})
	validatingWebhook, err := apiCl.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, mutatingWebhookStatus, err
	}
	mutatingWebhook, err := apiCl.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return validatingWebhookStatus, mutatingWebhookStatus, err
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
				path = fmt.Sprintf("Path: %s", *svc.Path)
			}
			validatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			validatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			validatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		validatingWebhooksConfig[w.Name]["CA bundle digest"] = getDigest(w.ClientConfig.CABundle)
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
				path = fmt.Sprintf("Path: %s", *svc.Path)
			}
			mutatingWebhooksConfig[w.Name]["Service"] = fmt.Sprintf("%s/%s - %s - %s", svc.Namespace, svc.Name, port, path)
		}
		if w.ObjectSelector != nil {
			mutatingWebhooksConfig[w.Name]["Object selector"] = w.ObjectSelector.String()
		}
		for i, r := range w.Rules {
			mutatingWebhooksConfig[w.Name][fmt.Sprintf("Rule %d", i+1)] = fmt.Sprintf("Operations: %v - APIGroups: %v - APIVersions: %v - Resources: %v", r.Operations, r.Rule.APIGroups, r.Rule.APIVersions, r.Rule.Resources)
		}
		mutatingWebhooksConfig[w.Name]["CA bundle digest"] = getDigest(w.ClientConfig.CABundle)
	}
	return validatingWebhookStatus, mutatingWebhookStatus, nil
}

func getSecretStatus(ns, name string, apiCl kubernetes.Interface) (map[string]interface{}, error) {
	secretStatus := make(map[string]interface{})
	secret, err := apiCl.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return secretStatus, err
	}
	secretStatus["Name"] = secret.GetName()
	secretStatus["Namespace"] = secret.GetNamespace()
	secretStatus["CreatedAt"] = secret.GetCreationTimestamp()
	secretStatus["CABundleDigest"] = getDigest(secret.Data["cert.pem"])
	cert, err := certificate.GetCertFromSecret(secret.Data)
	if err != nil {
		log.Errorf("Cannot get certificate from secret: %v", err)
	}
	t := certificate.GetDurationBeforeExpiration(cert)
	secretStatus["CertValidDuration"] = t.String()
	return secretStatus, nil
}

func getDigest(b []byte) string {
	h := fnv.New64()
	_, _ = h.Write(b)
	return strconv.FormatUint(h.Sum64(), 16)
}

// Provider provides the functionality to populate the status output
type Provider struct{}

//go:embed status_templates
var templatesFS embed.FS

// Name returns the name
func (Provider) Name() string {
	return "Admission Controller"
}

// Section return the section
func (Provider) Section() string {
	return "Admission Controller"
}

// JSON populates the status map
func (Provider) JSON(_ bool, stats map[string]interface{}) error {
	populateStatus(stats)

	return nil
}

// Text renders the text output
func (Provider) Text(_ bool, buffer io.Writer) error {
	return status.RenderText(templatesFS, "admissionwebhook.tmpl", buffer, getStatusInfo())
}

// HTML renders the html output
func (Provider) HTML(_ bool, _ io.Writer) error {
	return nil
}

func populateStatus(stats map[string]interface{}) {
	apiCl, apiErr := apiserver.GetAPIClient()
	if apiErr != nil {
		stats["admissionWebhook"] = map[string]string{"Error": apiErr.Error()}
	} else {
		stats["admissionWebhook"] = GetStatus(apiCl.Cl)
	}
}

func getStatusInfo() map[string]interface{} {
	stats := make(map[string]interface{})

	populateStatus(stats)

	return stats
}
